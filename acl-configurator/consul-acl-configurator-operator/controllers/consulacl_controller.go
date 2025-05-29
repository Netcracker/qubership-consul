// Copyright 2024-2025 NetCracker Technology Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Netcracker/consul-acl-configurator/consul-acl-configurator-operator/util"
	consulApi "github.com/hashicorp/consul/api"
	"k8s.io/apimachinery/pkg/api/errors"
	"net"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	consulacl "github.com/Netcracker/consul-acl-configurator/consul-acl-configurator-operator/api/v1alpha1"
)

const (
	consulAclFinalizer = "qubership.org/consulaclconfigurator-controller"
	errNotFound        = "ACL not found"
)

var log = logf.Log.WithName("controller_consulacl")

var ConsulClientService = os.Getenv("CONSUL_HOST")
var ConsulClientPort = os.Getenv("CONSUL_PORT")
var ConsulClientScheme = os.Getenv("CONSUL_SCHEME")
var bootstrapToken = os.Getenv("CONSUL_ACL_BOOTSTRAP_TOKEN")
var authMethod = os.Getenv("CONSUL_AUTH_METHOD_NAME")
var periodTime, _ = strconv.Atoi(os.Getenv("RECONCILE_PERIOD_SECONDS"))
var aclClient = makeAclClient()

// ConsulACLReconciler reconciles a ConsulACL object
type ConsulACLReconciler struct {
	Client           client.Client
	Scheme           *runtime.Scheme
	ResourceVersions map[string]string
}

//+kubebuilder:rbac:groups=qubership.org,resources=consulacls,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=qubership.org,resources=consulacls/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=qubership.org,resources=consulacls/finalizers,verbs=update

func (r *ConsulACLReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ConsulACL")

	// Fetch the ConsulACL instance
	instance := &consulacl.ConsulACL{}
	err := r.Client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	crUpdater := util.NewCustomResourceUpdater(r.Client, instance)
	if instance.DeletionTimestamp.IsZero() {
		if !util.Contains(consulAclFinalizer, instance.GetFinalizers()) {
			err = crUpdater.UpdateWithRetry(func(cr *consulacl.ConsulACL) {
				controllerutil.AddFinalizer(cr, consulAclFinalizer)
			})
			if err != nil {
				return reconcile.Result{}, err
			}
		}
	} else {
		if util.Contains(consulAclFinalizer, instance.GetFinalizers()) {
			return r.deleteACL(instance, crUpdater)
		}
		return reconcile.Result{}, nil
	}

	policiesStatus, rolesStatus, bindRulesStatus, err := r.applyACL(instance)
	if err != nil {
		if _, ok := err.(net.Error); ok {
			log.Error(err, "Error during connection to Consul")
		} else {
			log.Error(err, "Can not parse ACL configuration")
		}
		return reconcile.Result{RequeueAfter: time.Second * time.Duration(periodTime)}, nil
	}

	err = crUpdater.UpdateStatusWithRetry(func(cr *consulacl.ConsulACL) {
		cr.Status.PoliciesStatus = policiesStatus
		cr.Status.RolesStatus = rolesStatus
		cr.Status.BindRulesStatus = bindRulesStatus
	})
	if err != nil {
		log.Error(err, "Error occurred during custom resource status update")
		return reconcile.Result{RequeueAfter: time.Second * time.Duration(periodTime)}, nil
	}

	reqLogger.Info("Reconcile cycle succeeded")
	return reconcile.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ConsulACLReconciler) SetupWithManager(mgr ctrl.Manager) error {
	statusPredicate := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			// Ignore updates to CR status in which case metadata.Generation does not change
			return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// Evaluates to false if the object has been confirmed deleted.
			return !e.DeleteStateUnknown
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&consulacl.ConsulACL{}, builder.WithPredicates(statusPredicate)).
		Complete(r)
}

func (r *ConsulACLReconciler) deleteACL(instance *consulacl.ConsulACL, crUpdater util.CustomResourceUpdater) (ctrl.Result, error) {
	aclConfig, err := getAclConfig(instance)
	if err != nil {
		return ctrl.Result{}, err
	}
	err = r.deleteAclEntities(aclConfig, instance.Name, instance.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = crUpdater.UpdateWithRetry(func(cr *consulacl.ConsulACL) {
		controllerutil.RemoveFinalizer(cr, consulAclFinalizer)
	})
	return ctrl.Result{}, err
}

func (r *ConsulACLReconciler) deleteAclEntities(aclConfig *ACLConfig, name string, namespace string) error {
	if err := deleteBindingRules(aclConfig, name, namespace); err != nil {
		return err
	}
	if err := deleteRoles(aclConfig, name, namespace); err != nil {
		return err
	}
	if err := deletePolicies(aclConfig, name, namespace); err != nil {
		return err
	}
	log.Info(fmt.Sprintf("All ACL entities for ConsulACL resource with name - [%s] from namespace - [%s] are deleted",
		name, namespace))
	return nil
}

func deleteBindingRules(aclConfig *ACLConfig, name string, namespace string) error {
	existedBindingRules, _, err := aclClient.BindingRuleList(authMethod, &consulApi.QueryOptions{})
	if err != nil {
		return err
	}
	for _, br := range aclConfig.BindRules {
		for _, ebr := range existedBindingRules {
			if convertEntityName(br.BindName, name, namespace) == ebr.BindName {
				_, err = aclClient.BindingRuleDelete(ebr.ID, &consulApi.WriteOptions{})
				if err != nil {
					log.Error(err, fmt.Sprintf("Error occurred during binding rule deleting operation, binding rule id is [%s]", ebr.ID))
					return err
				}
			}
		}
	}
	return nil
}

func deleteRoles(aclConfig *ACLConfig, name string, namespace string) error {
	roles := aclConfig.Roles
	for _, role := range roles {
		roleName := convertEntityName(role.Name, name, namespace)
		deletedRole, err := readRole(roleName)
		if err != nil {
			log.Error(err, fmt.Sprintf("Error occurred during role reading operation, role name is [%s]", roleName))
			return err
		} else if deletedRole == nil {
			// skip deleting non-existent role
			continue
		}
		_, err = aclClient.RoleDelete(deletedRole.ID, &consulApi.WriteOptions{})
		if err != nil {
			log.Error(err, fmt.Sprintf("Error occurred during role deleting operation, role id is [%s]", deletedRole.ID))
			return err
		}
	}
	return nil
}

func deletePolicies(aclConfig *ACLConfig, name string, namespace string) error {
	policies := aclConfig.Policies
	for _, policy := range policies {
		policyName := convertEntityName(policy.Name, name, namespace)
		deletedPolicy, err := readPolicy(policyName)
		if err != nil {
			log.Error(err, fmt.Sprintf("Error occurred during policy reading operation, policy name is [%s]", policyName))
			return err
		} else if deletedPolicy == nil {
			// skip deleting non-existent policy
			continue
		}
		_, err = aclClient.PolicyDelete(deletedPolicy.ID, &consulApi.WriteOptions{})
		if err != nil {
			log.Error(err, fmt.Sprintf("Error occurred during policy deleting operation, policy id is [%s]", deletedPolicy.ID))
			return err
		}
	}
	return nil
}

func convertEntityName(entityName string, name string, namespace string) string {
	return fmt.Sprintf("%s_%s_%s", name, namespace, entityName)
}

func (r *ConsulACLReconciler) applyACL(cr *consulacl.ConsulACL) (string, string, string, error) {
	customResourceName := cr.Name
	customResourceNamespace := cr.Namespace
	aclConfig, err := getAclConfig(cr)
	if err != nil {
		return "", "", "", err
	}
	policiesStatus, processedPolicies, err := processPolicies(aclConfig.Policies, customResourceName, customResourceNamespace)
	if err != nil {
		return "", "", "", err
	}
	rolesStatus, err := processRoles(aclConfig.Roles, processedPolicies, customResourceName, customResourceNamespace)
	if err != nil {
		return "", "", "", err
	}
	bindRulesStatus, err := processBindRules(aclConfig.BindRules, customResourceName, customResourceNamespace)
	if err != nil {
		return "", "", "", err
	}
	return policiesStatus.GetStatus(), rolesStatus.GetStatus(), bindRulesStatus.GetStatus(), nil
}

func getAclConfig(cr *consulacl.ConsulACL) (*ACLConfig, error) {
	jsonField := cr.Spec.ACL.Json
	aclConfig := ACLConfig{}
	jsonBytes := []byte(jsonField)
	err := json.Unmarshal(jsonBytes, &aclConfig)
	if err != nil {
		return nil, err
	}
	return &aclConfig, nil
}

func processPolicies(policies []consulApi.ACLPolicy, customResourceName string, customResourceNamespace string) (*StatusHolder, map[string]string, error) {
	statusMap := StatusHolder{}
	processedPolicies := map[string]string{}
	var err error
	for _, policyDemand := range policies {
		if policyDemand.Name == "" {
			statusMap["innerErrorHandlingItem"] = "Some policies have not got a name"
			continue
		} else {
			policyDemand.Name = fmt.Sprintf("%s_%s_%s", customResourceName, customResourceNamespace, policyDemand.Name)
		}
		var resPolicy *consulApi.ACLPolicy
		var action string

		if policyDemand.ID == "" {
			resPolicy, err = readPolicy(policyDemand.Name)
			if err != nil {
				log.Info(fmt.Sprintf("Error occurred during reading a policy by name - %s, %s", policyDemand.Name, err.Error()))
			} else if resPolicy != nil {
				policyDemand.ID = resPolicy.ID
			}
		}

		if policyDemand.ID == "" {
			action = "create"
			resPolicy, _, err = aclClient.PolicyCreate(&policyDemand, &consulApi.WriteOptions{})
		} else {
			action = "update"
			resPolicy, _, err = aclClient.PolicyUpdate(&policyDemand, &consulApi.WriteOptions{})
		}

		if err != nil {
			log.Error(err, fmt.Sprintf("Can not %s a policy", action))
			statusMap[policyDemand.Name] = fmt.Sprintf("error: %s", err)
		} else {
			processedPolicies[policyDemand.Name] = resPolicy.ID
			statusMap[policyDemand.Name] = fmt.Sprintf("%sd", action)
		}
	}
	//Set error to nil in case we didn't receive any Network errors, other errors were logged previously
	if _, ok := err.(net.Error); !ok {
		err = nil
	}
	return &statusMap, processedPolicies, err
}

func processRoles(roles []ACLRoleAdapter, policies map[string]string, customResourceName string, customResourceNamespace string) (*StatusHolder, error) {
	statusMap := StatusHolder{}
	var err error
	for _, roleAdapter := range roles {
		if roleAdapter.Name == "" {
			statusMap["innerErrorHandlingItem"] = "Some roles have not got a name"
			continue
		}
		var resRole *consulApi.ACLRole
		var action string
		role := convertRoleAdapterToRole(roleAdapter, policies, customResourceName, customResourceNamespace)

		if role.ID == "" {
			resRole, err = readRole(role.Name)
			if err != nil {
				log.Info(fmt.Sprintf("Error occurred during reading a role by name - %s, %s", role.Name, err.Error()))
			} else if resRole != nil {
				role.ID = resRole.ID
			}
		}

		if role.ID == "" {
			action = "create"
			_, _, err = aclClient.RoleCreate(&role, &consulApi.WriteOptions{})
		} else {
			action = "update"
			_, _, err = aclClient.RoleUpdate(&role, &consulApi.WriteOptions{})
		}

		if err != nil {
			log.Error(err, fmt.Sprintf("can not %s a role", action))
			statusMap[role.Name] = fmt.Sprintf("error: %s", err)
		} else {
			statusMap[role.Name] = fmt.Sprintf("%sd", action)
		}
	}
	//Set error to nil in case we didn't receive any Network errors, other errors were logged previously
	if _, ok := err.(net.Error); !ok {
		err = nil
	}
	return &statusMap, err
}

func convertRoleAdapterToRole(roleAdapter ACLRoleAdapter, policies map[string]string, customResourceName string, customResourceNamespace string) consulApi.ACLRole {
	role := consulApi.ACLRole{}
	role.ID = roleAdapter.ID
	role.Name = fmt.Sprintf("%s_%s_%s", customResourceName, customResourceNamespace, roleAdapter.Name)
	role.Description = roleAdapter.Description
	role.Policies = getPolicyLinks(roleAdapter, policies, customResourceName, customResourceNamespace)
	return role
}

func getPolicyLinks(roleAdapter ACLRoleAdapter, policies map[string]string, customResourceName string, customResourceNamespace string) []*consulApi.ACLRolePolicyLink {
	var resLinks []*consulApi.ACLRolePolicyLink
	for _, policyName := range roleAdapter.PolicyNames {
		if policyID, ok := policies[fmt.Sprintf("%s_%s_%s", customResourceName, customResourceNamespace, policyName)]; ok {
			policyLink := consulApi.ACLRolePolicyLink{}
			policyLink.Name = fmt.Sprintf("%s_%s_%s", customResourceName, customResourceNamespace, policyName)
			policyLink.ID = policyID
			resLinks = append(resLinks, &policyLink)
		}
	}
	return resLinks
}

func processBindRules(bindRules []ACLBindingRuleAdapter, customResourceName string, customResourceNamespace string) (*StatusHolder, error) {
	statusMap := StatusHolder{}
	var err error
	//TODO: bind rule created every time with new id and maybe similar other fields
	for _, bindRuleAdapter := range bindRules {
		if bindRuleAdapter.BindName == "" {
			statusMap["innerErrorHandlingItem"] = "Some binding rules have not got a name"
			continue
		}
		bindRuleDemand := convertBindRuleAdapterToBindRule(bindRuleAdapter, customResourceName, customResourceNamespace)
		var action string
		if bindRuleDemand.ID == "" {
			_, _, err = aclClient.BindingRuleCreate(&bindRuleDemand, &consulApi.WriteOptions{})
			action = "create"
		} else {
			_, _, err = aclClient.BindingRuleUpdate(&bindRuleDemand, &consulApi.WriteOptions{})
			action = "update"
		}
		if err != nil {
			log.Error(err, fmt.Sprintf("can not %s a bind rule", action))
			statusMap[bindRuleDemand.BindName] = fmt.Sprintf("error: %s", err)
		} else {
			statusMap[fmt.Sprintf("Bind rule for %s with name %s",
				bindRuleDemand.BindType, bindRuleDemand.BindName)] = fmt.Sprintf("%sd", action)
		}
	}
	//Set error to nil in case we didn't receive any Network errors, other errors were logged previously
	if _, ok := err.(net.Error); !ok {
		err = nil
	}
	return &statusMap, err
}

func convertBindRuleAdapterToBindRule(bindRuleAdapter ACLBindingRuleAdapter, customResourceName string, customResourceNamespace string) consulApi.ACLBindingRule {
	bindingRule := consulApi.ACLBindingRule{}
	bindingRule.ID = bindRuleAdapter.ID
	bindingRule.BindName = fmt.Sprintf("%s_%s_%s", customResourceName, customResourceNamespace, bindRuleAdapter.BindName)
	bindingRule.BindType = "role"
	bindingRule.AuthMethod = authMethod
	bindingRule.Description = bindRuleAdapter.Description
	bindingRule.Selector = fmt.Sprintf("serviceaccount.namespace==\"%s\" and serviceaccount.name==\"%s\"",
		customResourceNamespace,
		bindRuleAdapter.ServiceAccountName)
	return bindingRule
}

func readRole(roleName string) (*consulApi.ACLRole, error) {
	role, _, err := aclClient.RoleReadByName(roleName, &consulApi.QueryOptions{})
	if role == nil || isErrNotFound(err) {
		log.Info(fmt.Sprintf("There is no role with name %s", roleName))
		return role, nil
	}
	return role, err
}

func readPolicy(policyName string) (*consulApi.ACLPolicy, error) {
	policy, _, err := aclClient.PolicyReadByName(policyName, &consulApi.QueryOptions{})
	if policy == nil || isErrNotFound(err) {
		log.Info(fmt.Sprintf("There is no policy with name %s", policyName))
		return policy, nil
	}
	return policy, err
}

func isErrNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), errNotFound)
}
