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
	"path/filepath"
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

const errNotFound = "ACL not found"

const podSecretsDir = "/etc/secrets/consul-acl-configurator-pod-secrets"
const bootstrapTokenEnv = "CONSUL_ACL_BOOTSTRAP_TOKEN"

var consulAclFinalizer = consulacl.GroupVersion.Group + "/consulaclconfigurator-controller"

var log = logf.Log.WithName("controller_consulacl")

var ConsulClientService = os.Getenv("CONSUL_HOST")
var ConsulClientPort = os.Getenv("CONSUL_PORT")
var ConsulClientScheme = os.Getenv("CONSUL_SCHEME")
var bootstrapToken = util.GetSecretFromFileOrEnv(
	filepath.Join(podSecretsDir, bootstrapTokenEnv),
	bootstrapTokenEnv,
)
var authMethod = os.Getenv("CONSUL_AUTH_METHOD_NAME")
var periodTime, _ = strconv.Atoi(os.Getenv("RECONCILE_PERIOD_SECONDS"))
var aclClient consulACLClient = makeAclClient()

// ConsulACLReconciler reconciles a ConsulACL object
type ConsulACLReconciler struct {
	Client           client.Client
	Scheme           *runtime.Scheme
	ResourceVersions map[string]string
	OwnNamespace     string
}

//+kubebuilder:rbac:groups=netcracker.com,resources=consulacls,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=netcracker.com,resources=consulacls/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=netcracker.com,resources=consulacls/finalizers,verbs=update

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

	policiesStatus, rolesStatus, bindRulesStatus, authMethodsStatus, err := r.applyACL(instance)
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
		cr.Status.AuthMethodsStatus = authMethodsStatus
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

	ownerPredicate := predicate.NewPredicateFuncs(func(obj client.Object) bool {
		cr, ok := obj.(*consulacl.ConsulACL)
		if !ok {
			return true
		}
		operatorNs := cr.Spec.ACL.OperatorNamespace
		if operatorNs != "" {
			return operatorNs == r.OwnNamespace
		}
		return obj.GetNamespace() == r.OwnNamespace
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&consulacl.ConsulACL{}, builder.WithPredicates(statusPredicate, ownerPredicate)).
		Complete(r)
}

func (r *ConsulACLReconciler) deleteACL(instance *consulacl.ConsulACL, crUpdater util.CustomResourceUpdater) (ctrl.Result, error) {
	aclConfig, err := getAclConfig(instance)
	if err != nil {
		log.Error(err, "Can not parse ACL configuration during deletion, skipping Consul cleanup and removing finalizer")
	} else {
		if err = r.deleteAclEntities(aclConfig, instance.Name, instance.Namespace, instance.Spec.ACL.ExplicitName); err != nil {
			return ctrl.Result{}, err
		}
	}

	err = crUpdater.UpdateWithRetry(func(cr *consulacl.ConsulACL) {
		controllerutil.RemoveFinalizer(cr, consulAclFinalizer)
	})
	return ctrl.Result{}, err
}

func (r *ConsulACLReconciler) deleteAclEntities(aclConfig *ACLConfig, name string, namespace string, explicitName bool) error {
	if err := deleteBindingRules(aclConfig, name, namespace, explicitName); err != nil {
		return err
	}
	if err := deleteRoles(aclConfig, name, namespace, explicitName); err != nil {
		return err
	}
	if err := deletePolicies(aclConfig, name, namespace, explicitName); err != nil {
		return err
	}
	if err := deleteAuthMethods(aclConfig, name, namespace, explicitName); err != nil {
		return err
	}
	log.Info(fmt.Sprintf("All ACL entities for ConsulACL resource with name - [%s] from namespace - [%s] are deleted",
		name, namespace))
	return nil
}

func deleteBindingRules(aclConfig *ACLConfig, name string, namespace string, explicitName bool) error {
	// Collect all distinct auth methods referenced in this CR (global + any per-rule overrides).
	authMethods := map[string]struct{}{authMethod: {}}
	for _, br := range aclConfig.BindRules {
		if br.AuthMethod != "" {
			authMethods[br.AuthMethod] = struct{}{}
		}
	}

	// Build the set of bind names declared by this CR.
	declaredBindNames := map[string]struct{}{}
	for _, br := range aclConfig.BindRules {
		var bindName string
		if explicitName {
			bindName = br.BindName
		} else {
			bindName = convertEntityName(br.BindName, name, namespace)
		}
		declaredBindNames[bindName] = struct{}{}
	}

	for am := range authMethods {
		existingRules, _, err := aclClient.BindingRuleList(am, &consulApi.QueryOptions{})
		if err != nil {
			return err
		}
		for _, ebr := range existingRules {
			if _, declared := declaredBindNames[ebr.BindName]; declared {
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

func deleteRoles(aclConfig *ACLConfig, name string, namespace string, explicitName bool) error {
	roles := aclConfig.Roles
	for _, role := range roles {
		var roleName string
		if explicitName {
			roleName = role.Name
		} else {
			roleName = convertEntityName(role.Name, name, namespace)
		}
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

func deletePolicies(aclConfig *ACLConfig, name string, namespace string, explicitName bool) error {
	policies := aclConfig.Policies
	for _, policy := range policies {
		var policyName string
		if explicitName {
			policyName = policy.Name
		} else {
			policyName = convertEntityName(policy.Name, name, namespace)
		}
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

func (r *ConsulACLReconciler) applyACL(cr *consulacl.ConsulACL) (string, string, string, string, error) {
	customResourceName := cr.Name
	customResourceNamespace := cr.Namespace
	aclConfig, err := getAclConfig(cr)
	if err != nil {
		return "", "", "", "", err
	}
	authMethodsStatus, err := processAuthMethods(aclConfig.AuthMethods, customResourceName, customResourceNamespace, cr.Spec.ACL.ExplicitName)
	if err != nil {
		return "", "", "", "", err
	}
	policiesStatus, processedPolicies, err := processPolicies(aclConfig.Policies, customResourceName, customResourceNamespace, cr.Spec.ACL.ExplicitName)
	if err != nil {
		return "", "", "", "", err
	}
	rolesStatus, err := processRoles(aclConfig.Roles, processedPolicies, customResourceName, customResourceNamespace, cr.Spec.ACL.ExplicitName)
	if err != nil {
		return "", "", "", "", err
	}
	bindRulesStatus, err := processBindRules(aclConfig.BindRules, customResourceName, customResourceNamespace, cr.Spec.ACL.ExplicitName)
	if err != nil {
		return "", "", "", "", err
	}
	if err := removeStaleEntities(aclConfig, customResourceName, customResourceNamespace, cr.Spec.ACL.ExplicitName); err != nil {
		return "", "", "", "", err
	}
	return policiesStatus.GetStatus(), rolesStatus.GetStatus(), bindRulesStatus.GetStatus(), authMethodsStatus.GetStatus(), nil
}

// removeStaleEntities deletes Consul entities that were present under this CR's naming
// pattern but are no longer declared in the current spec. Deletion order mirrors
// deleteAclEntities: binding rules → roles → policies.
func removeStaleEntities(aclConfig *ACLConfig, name string, namespace string, explicitName bool) error {
	namePrefix := fmt.Sprintf("%s_%s_", name, namespace)
	isOwned := func(entityName string) bool {
		if explicitName {
			return true // explicit names: we own whatever is in the spec
		}
		return strings.HasPrefix(entityName, namePrefix)
	}

	// --- binding rules ---
	// Collect all auth methods referenced in the current spec (global + per-rule overrides).
	authMethods := map[string]struct{}{authMethod: {}}
	for _, br := range aclConfig.BindRules {
		if br.AuthMethod != "" {
			authMethods[br.AuthMethod] = struct{}{}
		}
	}
	// Build the set of declared BindNames for this CR.
	declaredBindNames := map[string]struct{}{}
	for _, br := range aclConfig.BindRules {
		var bindName string
		if explicitName {
			bindName = br.BindName
		} else {
			bindName = convertEntityName(br.BindName, name, namespace)
		}
		declaredBindNames[bindName] = struct{}{}
	}
	for am := range authMethods {
		existingRules, _, err := aclClient.BindingRuleList(am, &consulApi.QueryOptions{})
		if err != nil {
			return err
		}
		for _, ebr := range existingRules {
			if !isOwned(ebr.BindName) {
				continue
			}
			if _, declared := declaredBindNames[ebr.BindName]; !declared {
				if _, err := aclClient.BindingRuleDelete(ebr.ID, &consulApi.WriteOptions{}); err != nil {
					log.Error(err, fmt.Sprintf("Error deleting stale binding rule [%s]", ebr.ID))
					return err
				}
			}
		}
	}

	// --- roles ---
	declaredRoleNames := map[string]struct{}{}
	for _, r := range aclConfig.Roles {
		var roleName string
		if explicitName {
			roleName = r.Name
		} else {
			roleName = convertEntityName(r.Name, name, namespace)
		}
		declaredRoleNames[roleName] = struct{}{}
	}
	existingRoles, _, err := aclClient.RoleList(&consulApi.QueryOptions{})
	if err != nil {
		return err
	}
	for _, er := range existingRoles {
		if !isOwned(er.Name) {
			continue
		}
		if _, declared := declaredRoleNames[er.Name]; !declared {
			if _, err := aclClient.RoleDelete(er.ID, &consulApi.WriteOptions{}); err != nil {
				log.Error(err, fmt.Sprintf("Error deleting stale role [%s]", er.ID))
				return err
			}
		}
	}

	// --- policies ---
	if !explicitName {
		declaredPolicyNames := map[string]struct{}{}
		for _, p := range aclConfig.Policies {
			declaredPolicyNames[convertEntityName(p.Name, name, namespace)] = struct{}{}
		}
		existingPolicies, _, err := aclClient.PolicyList(&consulApi.QueryOptions{})
		if err != nil {
			return err
		}
		for _, ep := range existingPolicies {
			if !strings.HasPrefix(ep.Name, namePrefix) {
				continue
			}
			if _, declared := declaredPolicyNames[ep.Name]; !declared {
				if _, err := aclClient.PolicyDelete(ep.ID, &consulApi.WriteOptions{}); err != nil {
					log.Error(err, fmt.Sprintf("Error deleting stale policy [%s]", ep.ID))
					return err
				}
			}
		}
	}

	return nil
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

func processPolicies(policies []consulApi.ACLPolicy, customResourceName string, customResourceNamespace string, explicitName bool) (*StatusHolder, map[string]string, error) {
	statusMap := StatusHolder{}
	processedPolicies := map[string]string{}
	var err error
	for _, policyDemand := range policies {
		if policyDemand.Name == "" {
			statusMap["innerErrorHandlingItem"] = "Some policies have not got a name"
			continue
		} else if !explicitName {
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

func processRoles(roles []ACLRoleAdapter, policies map[string]string, customResourceName string, customResourceNamespace string, explicitName bool) (*StatusHolder, error) {
	statusMap := StatusHolder{}
	var err error
	for _, roleAdapter := range roles {
		if roleAdapter.Name == "" {
			statusMap["innerErrorHandlingItem"] = "Some roles have not got a name"
			continue
		}
		var resRole *consulApi.ACLRole
		var action string
		role := convertRoleAdapterToRole(roleAdapter, policies, customResourceName, customResourceNamespace, explicitName)

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

func convertRoleAdapterToRole(roleAdapter ACLRoleAdapter, policies map[string]string, customResourceName string, customResourceNamespace string, explicitName bool) consulApi.ACLRole {
	role := consulApi.ACLRole{}
	role.ID = roleAdapter.ID
	if explicitName {
		role.Name = roleAdapter.Name
	} else {
		role.Name = convertEntityName(roleAdapter.Name, customResourceName, customResourceNamespace)
	}
	role.Description = roleAdapter.Description
	role.Policies = getPolicyLinks(roleAdapter, policies, customResourceName, customResourceNamespace, explicitName)
	return role
}

func getPolicyLinks(roleAdapter ACLRoleAdapter, policies map[string]string, customResourceName string, customResourceNamespace string, explicitName bool) []*consulApi.ACLRolePolicyLink {
	var resLinks []*consulApi.ACLRolePolicyLink
	for _, policyName := range roleAdapter.PolicyNames {
		var resolvedName string
		var resolvedID string
		if explicitName {
			resolvedName = policyName
			if id, ok := policies[policyName]; ok {
				resolvedID = id
			} else {
				// cross-CR reference: look up policy by exact name in Consul
				if p, err := readPolicy(policyName); err == nil && p != nil {
					resolvedID = p.ID
				}
			}
		} else {
			resolvedName = fmt.Sprintf("%s_%s_%s", customResourceName, customResourceNamespace, policyName)
			resolvedID = policies[resolvedName]
		}
		if resolvedID != "" {
			resLinks = append(resLinks, &consulApi.ACLRolePolicyLink{Name: resolvedName, ID: resolvedID})
		}
	}
	return resLinks
}

func processBindRules(bindRules []ACLBindingRuleAdapter, customResourceName string, customResourceNamespace string, explicitName bool) (*StatusHolder, error) {
	statusMap := StatusHolder{}
	var err error
	for _, bindRuleAdapter := range bindRules {
		if bindRuleAdapter.BindName == "" {
			statusMap["innerErrorHandlingItem"] = "Some binding rules have not got a name"
			continue
		}
		bindRuleDemand := convertBindRuleAdapterToBindRule(bindRuleAdapter, customResourceName, customResourceNamespace, explicitName)
		applicableAuthMethod := bindRuleDemand.AuthMethod
		existingRules, _, err := aclClient.BindingRuleList(applicableAuthMethod, &consulApi.QueryOptions{})
		if err != nil {
			return &statusMap, err
		}
		for _, existing := range existingRules {
			if existing.BindName == bindRuleDemand.BindName {
				bindRuleDemand.ID = existing.ID
				break
			}
		}
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

func convertBindRuleAdapterToBindRule(bindRuleAdapter ACLBindingRuleAdapter, customResourceName string, customResourceNamespace string, explicitName bool) consulApi.ACLBindingRule {
	bindingRule := consulApi.ACLBindingRule{}
	bindingRule.ID = bindRuleAdapter.ID
	if explicitName {
		bindingRule.BindName = bindRuleAdapter.BindName
	} else {
		bindingRule.BindName = convertEntityName(bindRuleAdapter.BindName, customResourceName, customResourceNamespace)
	}
	bindingRule.BindType = "role"
	if bindRuleAdapter.AuthMethod != "" {
		bindingRule.AuthMethod = bindRuleAdapter.AuthMethod
	} else {
		bindingRule.AuthMethod = authMethod
	}
	bindingRule.Description = bindRuleAdapter.Description
	if bindRuleAdapter.Selector != "" {
		bindingRule.Selector = bindRuleAdapter.Selector
	} else if bindRuleAdapter.ServiceAccountName != "" {
		bindingRule.Selector = fmt.Sprintf("serviceaccount.namespace==\"%s\" and serviceaccount.name==\"%s\"",
			customResourceNamespace,
			bindRuleAdapter.ServiceAccountName)
	}
	return bindingRule
}

func processAuthMethods(authMethods []ACLAuthMethodAdapter, customResourceName string, customResourceNamespace string, explicitName bool) (*StatusHolder, error) {
	statusMap := StatusHolder{}
	var err error
	for _, adapter := range authMethods {
		if adapter.Name == "" {
			statusMap["innerErrorHandlingItem"] = "Some auth methods have not got a name"
			continue
		}
		am := convertAuthMethodAdapterToAuthMethod(adapter, customResourceName, customResourceNamespace, explicitName)
		var action string
		existing, _, err := aclClient.AuthMethodRead(am.Name, &consulApi.QueryOptions{})
		if err != nil {
			log.Error(err, fmt.Sprintf("Error occurred during reading auth method by name - %s", am.Name))
			statusMap[am.Name] = fmt.Sprintf("error: %s", err)
			continue
		}
		if existing == nil {
			action = "create"
			_, _, err = aclClient.AuthMethodCreate(&am, &consulApi.WriteOptions{})
		} else {
			action = "update"
			_, _, err = aclClient.AuthMethodUpdate(&am, &consulApi.WriteOptions{})
		}
		if err != nil {
			log.Error(err, fmt.Sprintf("can not %s auth method", action))
			statusMap[am.Name] = fmt.Sprintf("error: %s", err)
		} else {
			statusMap[am.Name] = fmt.Sprintf("%sd", action)
		}
	}
	if _, ok := err.(net.Error); !ok {
		err = nil
	}
	return &statusMap, err
}

func convertAuthMethodAdapterToAuthMethod(adapter ACLAuthMethodAdapter, customResourceName string, customResourceNamespace string, explicitName bool) consulApi.ACLAuthMethod {
	am := consulApi.ACLAuthMethod{}
	if explicitName {
		am.Name = adapter.Name
	} else {
		am.Name = convertEntityName(adapter.Name, customResourceName, customResourceNamespace)
	}
	am.Type = adapter.Type
	am.Description = adapter.Description
	am.Config = adapter.Config
	return am
}

func deleteAuthMethods(aclConfig *ACLConfig, name string, namespace string, explicitName bool) error {
	for _, adapter := range aclConfig.AuthMethods {
		var amName string
		if explicitName {
			amName = adapter.Name
		} else {
			amName = convertEntityName(adapter.Name, name, namespace)
		}
		existing, _, err := aclClient.AuthMethodRead(amName, &consulApi.QueryOptions{})
		if err != nil || existing == nil {
			continue
		}
		if _, err := aclClient.AuthMethodDelete(amName, &consulApi.WriteOptions{}); err != nil {
			log.Error(err, fmt.Sprintf("Error occurred during auth method deleting operation, name is [%s]", amName))
			return err
		}
	}
	return nil
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
