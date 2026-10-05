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
	"bytes"
	"context"
	"encoding/json"
	goerrors "errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Netcracker/consul-acl-configurator/consul-acl-configurator-operator/util"
	consulApi "github.com/hashicorp/consul/api"
	"k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

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

// legacyAuthMethods are global auth methods used by earlier versions of the operator. Binding
// rules created by the operator under them are removed on reconcile and on deletion (migration).
var legacyAuthMethods = splitList(os.Getenv("CONSUL_LEGACY_AUTH_METHODS"))

// splitList splits a comma-separated value and drops empty items.
func splitList(value string) []string {
	var items []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}
	return items
}

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
	err := r.Client.Get(ctx, request.NamespacedName, instance)
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
			return r.deleteACL(ctx, instance, crUpdater)
		}
		return reconcile.Result{}, nil
	}

	status, applyErr := r.applyACL(ctx, instance)
	if applyErr != nil {
		if isRetryable(applyErr) {
			log.Error(applyErr, "Transient error of Consul, the request is requeued")
		} else {
			log.Error(applyErr, "Can not apply ACL configuration")
		}
	}

	statusErr := crUpdater.UpdateStatusWithRetry(func(cr *consulacl.ConsulACL) {
		// A stage that was not reached keeps the status of the previous reconcile.
		if status.policies != nil {
			cr.Status.PoliciesStatus = *status.policies
		}
		if status.roles != nil {
			cr.Status.RolesStatus = *status.roles
		}
		if status.bindRules != nil {
			cr.Status.BindRulesStatus = *status.bindRules
		}
		setSuccessfulCondition(&cr.Status.Conditions, applyErr, cr.Generation)
	})
	if statusErr != nil {
		log.Error(statusErr, "Error occurred during custom resource status update")
		return reconcile.Result{RequeueAfter: time.Second * time.Duration(periodTime)}, nil
	}

	if applyErr != nil {
		return reconcile.Result{RequeueAfter: time.Second * time.Duration(periodTime)}, nil
	}

	reqLogger.Info("Reconcile cycle succeeded")
	return reconcile.Result{}, nil
}

// setSuccessfulCondition sets the "Successful" status condition on the given conditions slice.
func setSuccessfulCondition(conditions *[]metav1.Condition, err error, generation int64) {
	c := metav1.Condition{
		Type:               "Successful",
		ObservedGeneration: generation,
		LastTransitionTime: metav1.Now(),
	}
	if err == nil {
		c.Status = metav1.ConditionTrue
		c.Reason = "Reconciled"
		c.Message = ""
	} else {
		c.Status = metav1.ConditionFalse
		c.Reason = "Failed"
		c.Message = err.Error()
	}
	apimeta.SetStatusCondition(conditions, c)
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
		return r.isManaged(cr)
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&consulacl.ConsulACL{}, builder.WithPredicates(statusPredicate, ownerPredicate)).
		Complete(r)
}

// isManaged reports whether the given ConsulACL resource is reconciled by this operator instance.
func (r *ConsulACLReconciler) isManaged(cr *consulacl.ConsulACL) bool {
	if cr.Spec.ACL != nil && cr.Spec.ACL.OperatorNamespace != "" {
		return cr.Spec.ACL.OperatorNamespace == r.OwnNamespace
	}
	return cr.GetNamespace() == r.OwnNamespace
}

// siblingEntities collects the Consul names of entities declared by the other ConsulACL
// resources of the same namespace managed by this operator. The owner list stores only
// namespaces, so an entity declared by a sibling must not be released by the given resource.
// Resources that are being deleted are not counted: they release their entities themselves.
//
// A sibling whose configuration can not be parsed does not fail the operation: its entities are
// unknown, so the result declares every entity (see declaredEntities.all) and nothing is released.
// The entities left this way are released by the stale clean-up of the sibling once it is fixed.
func (r *ConsulACLReconciler) siblingEntities(ctx context.Context, cr *consulacl.ConsulACL) (*declaredEntities, error) {
	list := &consulacl.ConsulACLList{}
	if err := r.Client.List(ctx, list, client.InNamespace(cr.Namespace)); err != nil {
		return nil, err
	}
	siblings := newDeclaredEntities()
	for i := range list.Items {
		sibling := &list.Items[i]
		if sibling.Name == cr.Name || !sibling.DeletionTimestamp.IsZero() || sibling.Spec.ACL == nil || !r.isManaged(sibling) {
			continue
		}
		siblingConfig, err := getAclConfig(sibling)
		if err != nil {
			log.Error(err, fmt.Sprintf("Can not parse ACL configuration of ConsulACL [%s] in namespace [%s]: "+
				"shared entities of ConsulACL [%s] are not released until it is fixed", sibling.Name, sibling.Namespace, cr.Name))
			siblings.all = true
			continue
		}
		siblings.add(siblingConfig, sibling.Name, sibling.Namespace, sibling.Spec.ACL.ExplicitName)
	}
	return siblings, nil
}

func (r *ConsulACLReconciler) deleteACL(ctx context.Context, instance *consulacl.ConsulACL, crUpdater util.CustomResourceUpdater) (ctrl.Result, error) {
	aclConfig, err := getAclConfig(instance)
	if err != nil {
		log.Error(err, "Can not parse ACL configuration during deletion; Consul entities may need manual cleanup. To force deletion, remove the finalizer manually.")
		return ctrl.Result{}, err
	}

	// Prefixed names ({crName}_{crNamespace}_...) belong to this resource only, the siblings are
	// needed only for explicit names.
	var siblings *declaredEntities
	if instance.Spec.ACL.ExplicitName {
		if siblings, err = r.siblingEntities(ctx, instance); err != nil {
			log.Error(err, "Can not list ConsulACL resources of the same namespace")
			return ctrl.Result{}, err
		}
	}

	if err = r.deleteAclEntities(aclConfig, instance.Name, instance.Namespace, instance.Spec.ACL.ExplicitName, siblings); err != nil {
		return ctrl.Result{}, err
	}

	err = crUpdater.UpdateWithRetry(func(cr *consulacl.ConsulACL) {
		controllerutil.RemoveFinalizer(cr, consulAclFinalizer)
	})
	return ctrl.Result{}, err
}

func (r *ConsulACLReconciler) deleteAclEntities(aclConfig *ACLConfig, name string, namespace string, explicitName bool, siblings *declaredEntities) error {
	if err := deleteBindingRules(aclConfig, name, namespace, explicitName, siblings); err != nil {
		return err
	}
	if err := removeLegacyBindingRules(aclConfig, name, namespace, false, nil); err != nil {
		return err
	}
	if err := deleteRoles(aclConfig, name, namespace, explicitName, siblings); err != nil {
		return err
	}
	if err := deletePolicies(aclConfig, name, namespace, explicitName, siblings); err != nil {
		return err
	}
	log.Info(fmt.Sprintf("All ACL entities for ConsulACL resource with name - [%s] from namespace - [%s] are deleted",
		name, namespace))
	return nil
}

// revokeRoleTokens revokes all Consul tokens associated with the given role.
// Errors are logged but do not block deletion.
func revokeRoleTokens(role *consulApi.ACLRole) {
	// The token list filter expects a role ID (UUID), not a name.
	tokens, _, err := aclClient.TokenListFiltered(consulApi.ACLTokenFilterOptions{Role: role.ID}, &consulApi.QueryOptions{})
	if err != nil {
		log.Error(err, "Error listing tokens for role", "role", role.Name)
		return
	}
	for _, token := range tokens {
		if _, err := aclClient.TokenDelete(token.AccessorID, &consulApi.WriteOptions{}); err != nil {
			log.Error(err, "Error revoking token", "accessorID", token.AccessorID, "role", role.Name)
		}
	}
}

// releaseBindingRule removes namespace from the owners of the binding rule and deletes
// the rule when no other owners remain.
func releaseBindingRule(rule *consulApi.ACLBindingRule, namespace string) error {
	newDesc, isLast := withOwnerRemoved(rule.Description, namespace)
	if !isLast {
		log.Info(fmt.Sprintf("Binding rule [%s] is still used by other services, removing own namespace from description", rule.BindName))
		rule.Description = newDesc
		if _, _, err := aclClient.BindingRuleUpdate(rule, &consulApi.WriteOptions{}); err != nil {
			log.Error(err, fmt.Sprintf("Error updating binding rule description for [%s]", rule.BindName))
			return err
		}
		return nil
	}
	if _, err := aclClient.BindingRuleDelete(rule.ID, &consulApi.WriteOptions{}); err != nil {
		log.Error(err, fmt.Sprintf("Error occurred during binding rule deleting operation, binding rule id is [%s]", rule.ID))
		return err
	}
	return nil
}

// releaseRole removes namespace from the owners of the role. When no other owners remain,
// the tokens of the role are revoked and the role is deleted.
func releaseRole(role *consulApi.ACLRole, namespace string) error {
	newDesc, isLast := withOwnerRemoved(role.Description, namespace)
	if !isLast {
		log.Info(fmt.Sprintf("Role [%s] is still used by other services, removing own namespace from description", role.Name))
		role.Description = newDesc
		if _, _, err := aclClient.RoleUpdate(role, &consulApi.WriteOptions{}); err != nil {
			log.Error(err, fmt.Sprintf("Error updating role description for [%s]", role.Name))
			return err
		}
		return nil
	}
	revokeRoleTokens(role)
	if _, err := aclClient.RoleDelete(role.ID, &consulApi.WriteOptions{}); err != nil {
		log.Error(err, fmt.Sprintf("Error occurred during role deleting operation, role id is [%s]", role.ID))
		return err
	}
	return nil
}

// releasePolicy removes namespace from the owners of the policy and deletes the policy
// when no other owners remain.
func releasePolicy(policy *consulApi.ACLPolicy, namespace string) error {
	newDesc, isLast := withOwnerRemoved(policy.Description, namespace)
	if !isLast {
		log.Info(fmt.Sprintf("Policy [%s] is still used by other services, removing own namespace from description", policy.Name))
		policy.Description = newDesc
		if _, _, err := aclClient.PolicyUpdate(policy, &consulApi.WriteOptions{}); err != nil {
			log.Error(err, fmt.Sprintf("Error updating policy description for [%s]", policy.Name))
			return err
		}
		return nil
	}
	if _, err := aclClient.PolicyDelete(policy.ID, &consulApi.WriteOptions{}); err != nil {
		log.Error(err, fmt.Sprintf("Error occurred during policy deleting operation, policy id is [%s]", policy.ID))
		return err
	}
	return nil
}

// failedEntities returns the names of the entities whose last write failed (status "error: ...").
func failedEntities(status *StatusHolder) map[string]bool {
	failed := map[string]bool{}
	for name, value := range *status {
		if strings.HasPrefix(value, "error:") {
			failed[name] = true
		}
	}
	return failed
}

// bindRuleAuthMethods returns all distinct auth methods referenced in the config (global + per-rule overrides).
func bindRuleAuthMethods(aclConfig *ACLConfig) map[string]struct{} {
	authMethods := map[string]struct{}{authMethod: {}}
	for _, br := range aclConfig.BindRules {
		if br.AuthMethod != "" {
			authMethods[br.AuthMethod] = struct{}{}
		}
	}
	return authMethods
}

// removeLegacyBindingRules deletes the binding rules of the CR left under legacy global auth
// methods by earlier versions of the operator. Earlier versions created only prefixed names
// ({crName}_{crNamespace}_...) with BindType "role", so only such rules are matched. When keepDeclared
// is set, a rule that the current spec still declares with that legacy method as its per-rule
// AuthMethod is kept. A rule whose name is in failed is kept too: its replacement under the current
// method was not written, and removing it would leave the service without the rule. The current
// global method is never treated as legacy.
func removeLegacyBindingRules(aclConfig *ACLConfig, name string, namespace string, keepDeclared bool, failed map[string]bool) error {
	prefix := fmt.Sprintf("%s_%s_", name, namespace)
	for _, legacy := range legacyAuthMethods {
		if legacy == authMethod {
			continue
		}
		declared := map[string]struct{}{}
		if keepDeclared {
			for _, br := range aclConfig.BindRules {
				if br.AuthMethod == legacy && br.BindName != "" {
					declared[convertEntityName(br.BindName, name, namespace)] = struct{}{}
					declared[br.BindName] = struct{}{}
				}
			}
		}
		// The legacy method may not exist (for example connect-inject is disabled).
		method, _, err := aclClient.AuthMethodRead(legacy, &consulApi.QueryOptions{})
		if err != nil && !isErrNotFound(err) {
			return err
		}
		if method == nil {
			continue
		}
		rules, _, err := aclClient.BindingRuleList(legacy, &consulApi.QueryOptions{})
		if err != nil {
			return err
		}
		for _, rule := range rules {
			if rule.BindType != consulApi.BindingRuleBindTypeRole || !strings.HasPrefix(rule.BindName, prefix) {
				continue
			}
			if _, ok := declared[rule.BindName]; ok || failed[rule.BindName] {
				continue
			}
			if _, err = aclClient.BindingRuleDelete(rule.ID, &consulApi.WriteOptions{}); err != nil {
				log.Error(err, fmt.Sprintf("Error deleting binding rule [%s] under legacy auth method [%s]", rule.BindName, legacy))
				return err
			}
			log.Info(fmt.Sprintf("Deleted binding rule [%s] under legacy auth method [%s]", rule.BindName, legacy))
		}
	}
	return nil
}

func deleteBindingRules(aclConfig *ACLConfig, name string, namespace string, explicitName bool, siblings *declaredEntities) error {
	declared := newDeclaredEntities()
	declared.add(aclConfig, name, namespace, explicitName)

	for am := range bindRuleAuthMethods(aclConfig) {
		existingRules, _, err := aclClient.BindingRuleList(am, &consulApi.QueryOptions{})
		if err != nil {
			return err
		}
		for _, ebr := range existingRules {
			if !declared.hasBindRule(ebr.BindName) {
				continue
			}
			if siblings.hasBindRule(ebr.BindName) {
				log.Info(fmt.Sprintf("Binding rule [%s] is still declared by another resource of namespace [%s], skipping", ebr.BindName, namespace))
				continue
			}
			if err = releaseBindingRule(ebr, namespace); err != nil {
				return err
			}
		}
	}
	return nil
}

func deleteRoles(aclConfig *ACLConfig, name string, namespace string, explicitName bool, siblings *declaredEntities) error {
	for _, role := range aclConfig.Roles {
		if role.Name == "" {
			continue
		}
		roleName := resolveEntityName(role.Name, name, namespace, explicitName)
		if siblings.hasRole(roleName) {
			log.Info(fmt.Sprintf("Role [%s] is still declared by another resource of namespace [%s], skipping", roleName, namespace))
			continue
		}
		existingRole, err := readRole(roleName)
		if err != nil {
			log.Error(err, fmt.Sprintf("Error occurred during role reading operation, role name is [%s]", roleName))
			return err
		} else if existingRole == nil {
			// skip deleting non-existent role
			continue
		}
		if err = releaseRole(existingRole, namespace); err != nil {
			return err
		}
	}
	return nil
}

func deletePolicies(aclConfig *ACLConfig, name string, namespace string, explicitName bool, siblings *declaredEntities) error {
	for _, policy := range aclConfig.Policies {
		if policy.Name == "" {
			continue
		}
		policyName := resolveEntityName(policy.Name, name, namespace, explicitName)
		if siblings.hasPolicy(policyName) {
			log.Info(fmt.Sprintf("Policy [%s] is still declared by another resource of namespace [%s], skipping", policyName, namespace))
			continue
		}
		existingPolicy, err := readPolicy(policyName)
		if err != nil {
			log.Error(err, fmt.Sprintf("Error occurred during policy reading operation, policy name is [%s]", policyName))
			return err
		} else if existingPolicy == nil {
			continue
		}
		if err = releasePolicy(existingPolicy, namespace); err != nil {
			return err
		}
	}
	return nil
}

func convertEntityName(entityName string, name string, namespace string) string {
	return fmt.Sprintf("%s_%s_%s", name, namespace, entityName)
}

// resolveEntityName returns the Consul name of an entity declared by a ConsulACL resource.
func resolveEntityName(entityName string, name string, namespace string, explicitName bool) string {
	if explicitName {
		return entityName
	}
	return convertEntityName(entityName, name, namespace)
}

// declaredEntities holds the Consul names of ACL entities declared by one or more ConsulACL resources.
// A nil *declaredEntities is a valid empty set. With all set, every name is declared: it is used when
// the configuration of a sibling can not be parsed and its entities are unknown.
type declaredEntities struct {
	policies  map[string]struct{}
	roles     map[string]struct{}
	bindRules map[string]struct{}
	all       bool
}

func newDeclaredEntities() *declaredEntities {
	return &declaredEntities{
		policies:  map[string]struct{}{},
		roles:     map[string]struct{}{},
		bindRules: map[string]struct{}{},
	}
}

func (d *declaredEntities) add(aclConfig *ACLConfig, name string, namespace string, explicitName bool) {
	for _, p := range aclConfig.Policies {
		if p.Name != "" {
			d.policies[resolveEntityName(p.Name, name, namespace, explicitName)] = struct{}{}
		}
	}
	for _, r := range aclConfig.Roles {
		if r.Name != "" {
			d.roles[resolveEntityName(r.Name, name, namespace, explicitName)] = struct{}{}
		}
	}
	for _, br := range aclConfig.BindRules {
		if br.BindName != "" {
			d.bindRules[resolveEntityName(br.BindName, name, namespace, explicitName)] = struct{}{}
		}
	}
}

func (d *declaredEntities) hasPolicy(name string) bool {
	if d == nil {
		return false
	}
	if d.all {
		return true
	}
	_, ok := d.policies[name]
	return ok
}

func (d *declaredEntities) hasRole(name string) bool {
	if d == nil {
		return false
	}
	if d.all {
		return true
	}
	_, ok := d.roles[name]
	return ok
}

func (d *declaredEntities) hasBindRule(name string) bool {
	if d == nil {
		return false
	}
	if d.all {
		return true
	}
	_, ok := d.bindRules[name]
	return ok
}

// aclStatus holds the per-entity status strings of one reconcile. A nil field means that the stage
// was not reached, and the status of the previous reconcile is kept for it.
type aclStatus struct {
	policies, roles, bindRules *string
}

func statusString(status *StatusHolder) *string {
	s := status.GetStatus()
	return &s
}

// applyACL applies the configuration of the CR. The status of every stage that was processed is
// returned also on error, including the partial status of the stage that failed.
func (r *ConsulACLReconciler) applyACL(ctx context.Context, cr *consulacl.ConsulACL) (aclStatus, error) {
	var status aclStatus
	customResourceName := cr.Name
	customResourceNamespace := cr.Namespace
	aclConfig, err := getAclConfig(cr)
	if err != nil {
		return status, err
	}
	policiesStatus, processedPolicies, err := processPolicies(aclConfig.Policies, customResourceName, customResourceNamespace, cr.Spec.ACL.ExplicitName)
	status.policies = statusString(policiesStatus)
	if err != nil {
		return status, err
	}
	rolesStatus, err := processRoles(aclConfig.Roles, processedPolicies, customResourceName, customResourceNamespace, cr.Spec.ACL.ExplicitName)
	status.roles = statusString(rolesStatus)
	if err != nil {
		return status, err
	}
	bindRulesStatus, err := processBindRules(aclConfig.BindRules, customResourceName, customResourceNamespace, cr.Spec.ACL.ExplicitName)
	status.bindRules = statusString(bindRulesStatus)
	if err != nil {
		return status, err
	}
	// Migration: the rules now live under the current global method, drop the copies under legacy ones.
	if err = removeLegacyBindingRules(aclConfig, customResourceName, customResourceNamespace, true, failedEntities(bindRulesStatus)); err != nil {
		return status, err
	}
	var siblings *declaredEntities
	if cr.Spec.ACL.ExplicitName {
		if siblings, err = r.siblingEntities(ctx, cr); err != nil {
			return status, err
		}
	}
	if err = removeStaleEntities(aclConfig, customResourceName, customResourceNamespace, cr.Spec.ACL.ExplicitName, siblings); err != nil {
		return status, err
	}
	return status, nil
}

// removeStaleEntities deletes Consul entities that were present under this CR's naming
// pattern but are no longer declared in the current spec. Deletion order mirrors
// deleteAclEntities: binding rules → roles → policies.
//
// With explicitName=false, ownership is determined by the name prefix {crName}_{crNamespace}_.
//
// With explicitName=true, ownership is determined by the [consul-acl-owners:] list stored
// in the Description of each entity (see removeStaleExplicitEntities).
func removeStaleEntities(aclConfig *ACLConfig, name string, namespace string, explicitName bool, siblings *declaredEntities) error {
	if explicitName {
		return removeStaleExplicitEntities(aclConfig, name, namespace, siblings)
	}

	namePrefix := fmt.Sprintf("%s_%s_", name, namespace)
	isOwned := func(entityName string) bool {
		return strings.HasPrefix(entityName, namePrefix)
	}

	// --- binding rules ---
	// Build the set of declared BindNames for this CR.
	declaredBindNames := map[string]struct{}{}
	for _, br := range aclConfig.BindRules {
		declaredBindNames[convertEntityName(br.BindName, name, namespace)] = struct{}{}
	}
	for am := range bindRuleAuthMethods(aclConfig) {
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
		declaredRoleNames[convertEntityName(r.Name, name, namespace)] = struct{}{}
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
			if _, err = aclClient.RoleDelete(er.ID, &consulApi.WriteOptions{}); err != nil {
				log.Error(err, fmt.Sprintf("Error deleting stale role [%s]", er.ID))
				return err
			}
		}
	}

	// --- policies ---
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
			if _, err = aclClient.PolicyDelete(ep.ID, &consulApi.WriteOptions{}); err != nil {
				log.Error(err, fmt.Sprintf("Error deleting stale policy [%s]", ep.ID))
				return err
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

// removeStaleExplicitEntities handles stale cleanup when explicitName=true. The source of
// truth is the owner list in Consul, so cleanup does not depend on the CR status: an entity
// is stale for this CR when namespace is in its owner list and neither this CR nor another
// resource of the same namespace (siblings) declares it. Stale entities are released: the
// namespace is removed from the owner list, and the entity is deleted (the tokens of a role
// revoked) when no other owners remain. Entities without the owner marker are not touched.
//
// Binding rules are searched under the auth methods referenced in the current spec only.
func removeStaleExplicitEntities(aclConfig *ACLConfig, name string, namespace string, siblings *declaredEntities) error {
	declared := newDeclaredEntities()
	declared.add(aclConfig, name, namespace, true)

	// --- binding rules ---
	for am := range bindRuleAuthMethods(aclConfig) {
		existingRules, _, err := aclClient.BindingRuleList(am, &consulApi.QueryOptions{})
		if err != nil {
			return err
		}
		for _, ebr := range existingRules {
			if !hasOwner(ebr.Description, namespace) || declared.hasBindRule(ebr.BindName) || siblings.hasBindRule(ebr.BindName) {
				continue
			}
			log.Info(fmt.Sprintf("Releasing stale explicit binding rule [%s]", ebr.BindName))
			if err = releaseBindingRule(ebr, namespace); err != nil {
				return err
			}
		}
	}

	// --- roles ---
	existingRoles, _, err := aclClient.RoleList(&consulApi.QueryOptions{})
	if err != nil {
		return err
	}
	for _, er := range existingRoles {
		if !hasOwner(er.Description, namespace) || declared.hasRole(er.Name) || siblings.hasRole(er.Name) {
			continue
		}
		log.Info(fmt.Sprintf("Releasing stale explicit role [%s]", er.Name))
		if err = releaseRole(er, namespace); err != nil {
			return err
		}
	}

	// --- policies ---
	existingPolicies, _, err := aclClient.PolicyList(&consulApi.QueryOptions{})
	if err != nil {
		return err
	}
	for _, ep := range existingPolicies {
		if !hasOwner(ep.Description, namespace) || declared.hasPolicy(ep.Name) || siblings.hasPolicy(ep.Name) {
			continue
		}
		// The list entry has no rules, read the full policy so that an update keeps them.
		existingPolicy, err := readPolicy(ep.Name)
		if err != nil {
			return err
		}
		if existingPolicy == nil {
			continue
		}
		log.Info(fmt.Sprintf("Releasing stale explicit policy [%s]", ep.Name))
		if err = releasePolicy(existingPolicy, namespace); err != nil {
			return err
		}
	}
	return nil
}

const ownersMarker = "[consul-acl-owners:"

// parseOwners splits an entity description into the user-visible base text and
// the list of owner namespaces recorded by the operator.
func parseOwners(description string) (baseDesc string, owners []string) {
	idx := strings.LastIndex(description, ownersMarker)
	if idx == -1 {
		return strings.TrimRight(description, " \n"), nil
	}
	baseDesc = strings.TrimRight(description[:idx], " \n")
	raw := strings.TrimSuffix(strings.TrimSpace(description[idx+len(ownersMarker):]), "]")
	for _, o := range strings.Split(raw, ",") {
		if o = strings.TrimSpace(o); o != "" {
			owners = append(owners, o)
		}
	}
	return
}

// buildDescription reassembles base description and owner list into the full description string.
func buildDescription(baseDesc string, owners []string) string {
	if len(owners) == 0 {
		return baseDesc
	}
	sort.Strings(owners)
	suffix := fmt.Sprintf("%s %s]", ownersMarker, strings.Join(owners, ", "))
	if baseDesc == "" {
		return suffix
	}
	return baseDesc + "\n" + suffix
}

// withOwnerRemoved returns description with namespace removed from the owners list,
// and isLast=true when no other owners remain (caller should delete the policy).
func withOwnerRemoved(description, namespace string) (string, bool) {
	base, owners := parseOwners(description)
	var remaining []string
	for _, o := range owners {
		if o != namespace {
			remaining = append(remaining, o)
		}
	}
	return buildDescription(base, remaining), len(remaining) == 0
}

// hasOwner reports whether namespace is in the owner list of the description.
func hasOwner(description, namespace string) bool {
	_, owners := parseOwners(description)
	for _, o := range owners {
		if o == namespace {
			return true
		}
	}
	return false
}

// withOwnerAdded returns the spec description extended with the owners recorded in the
// existing Consul description and the given namespace.
func withOwnerAdded(specDescription, existingDescription, namespace string) string {
	specBase, _ := parseOwners(specDescription)
	_, owners := parseOwners(existingDescription)
	if !hasOwner(existingDescription, namespace) {
		owners = append(owners, namespace)
	}
	return buildDescription(specBase, owners)
}

// mergeOwnerIntoPolicy sets demand.Description to spec base + existing Consul owners + current namespace.
func mergeOwnerIntoPolicy(demand *consulApi.ACLPolicy, existing *consulApi.ACLPolicy, namespace string) {
	var existingDescription string
	if existing != nil {
		existingDescription = existing.Description
	}
	demand.Description = withOwnerAdded(demand.Description, existingDescription, namespace)
}

func processPolicies(policies []consulApi.ACLPolicy, customResourceName string, customResourceNamespace string, explicitName bool) (*StatusHolder, map[string]string, error) {
	statusMap := StatusHolder{}
	processedPolicies := map[string]string{}
	var err, retryErr error
	for _, policyDemand := range policies {
		if policyDemand.Name == "" {
			statusMap["innerErrorHandlingItem"] = "Some policies have not got a name"
			continue
		} else if !explicitName {
			policyDemand.Name = fmt.Sprintf("%s_%s_%s", customResourceName, customResourceNamespace, policyDemand.Name)
		}
		var resPolicy *consulApi.ACLPolicy
		var action string
		var existingPolicy *consulApi.ACLPolicy

		if policyDemand.ID == "" {
			existingPolicy, err = readPolicy(policyDemand.Name)
			if err != nil {
				// Without the read the operator can not tell create from update, nor keep the owners.
				log.Error(err, fmt.Sprintf("Can not read a policy by name - %s", policyDemand.Name))
				statusMap[policyDemand.Name] = fmt.Sprintf("error: %s", err)
				retryErr = firstRetryableError(retryErr, err)
				continue
			} else if existingPolicy != nil {
				policyDemand.ID = existingPolicy.ID
			}
		}

		mergeOwnerIntoPolicy(&policyDemand, existingPolicy, customResourceNamespace)

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
			retryErr = firstRetryableError(retryErr, err)
		} else {
			processedPolicies[policyDemand.Name] = resPolicy.ID
			statusMap[policyDemand.Name] = fmt.Sprintf("%sd", action)
		}
	}
	// Only transient errors are returned (the request is requeued), other errors were logged and recorded in the status
	return &statusMap, processedPolicies, retryErr
}

// retryableError marks an error that is not reported by Consul as transient but must be retried,
// for example an auth method that does not exist yet.
type retryableError struct{ error }

func (e retryableError) Unwrap() error { return e.error }

// isRetryable reports whether err is a transient failure after which the reconcile must be repeated:
// a network error, a 5xx or 429 response of Consul (no cluster leader, leadership lost, rate limit)
// or a retryableError.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if goerrors.As(err, &netErr) {
		return true
	}
	var statusErr consulApi.StatusError
	if goerrors.As(err, &statusErr) {
		return statusErr.Code >= http.StatusInternalServerError || statusErr.Code == http.StatusTooManyRequests
	}
	var retryErr retryableError
	return goerrors.As(err, &retryErr)
}

// firstRetryableError returns current if it is already set, otherwise err when it is retryable.
// It keeps the first transient error seen while processing a list of entities, so that a later
// successful call does not hide it.
func firstRetryableError(current, err error) error {
	if current != nil {
		return current
	}
	if isRetryable(err) {
		return err
	}
	return nil
}

func processRoles(roles []ACLRoleAdapter, policies map[string]string, customResourceName string, customResourceNamespace string, explicitName bool) (*StatusHolder, error) {
	statusMap := StatusHolder{}
	var err, retryErr error
	for _, roleAdapter := range roles {
		if roleAdapter.Name == "" {
			statusMap["innerErrorHandlingItem"] = "Some roles have not got a name"
			continue
		}
		var existingRole *consulApi.ACLRole
		var action string
		role := convertRoleAdapterToRole(roleAdapter, policies, customResourceName, customResourceNamespace, explicitName)

		// The existing role is read even when the ID is set in the spec, its owner list must be kept.
		existingRole, err = readRole(role.Name)
		if err != nil {
			// Without the read the operator can not tell create from update, nor keep the owners.
			log.Error(err, fmt.Sprintf("Can not read a role by name - %s", role.Name))
			statusMap[role.Name] = fmt.Sprintf("error: %s", err)
			retryErr = firstRetryableError(retryErr, err)
			continue
		} else if existingRole != nil && role.ID == "" {
			role.ID = existingRole.ID
		}
		var existingDescription string
		if existingRole != nil {
			existingDescription = existingRole.Description
		}
		role.Description = withOwnerAdded(role.Description, existingDescription, customResourceNamespace)

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
			retryErr = firstRetryableError(retryErr, err)
		} else {
			statusMap[role.Name] = fmt.Sprintf("%sd", action)
		}
	}
	// Only transient errors are returned (the request is requeued), other errors were logged and recorded in the status
	return &statusMap, retryErr
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
	var err, retryErr error
	authMethodType := authMethodTypeResolver()
	for _, bindRuleAdapter := range bindRules {
		if bindRuleAdapter.BindName == "" {
			statusMap["innerErrorHandlingItem"] = "Some binding rules have not got a name"
			continue
		}
		applicableAuthMethod := bindRuleAuthMethod(bindRuleAdapter)
		methodType, found, typeErr := authMethodType(applicableAuthMethod)
		if typeErr != nil {
			return &statusMap, typeErr
		}
		if !found {
			// Consul rejects a rule of an unknown method. The global method may be created a moment
			// later (at start-up), so the reconcile is repeated instead of being reported as successful.
			missingErr := retryableError{fmt.Errorf("auth method %q does not exist", applicableAuthMethod)}
			log.Error(missingErr, fmt.Sprintf("can not apply a bind rule [%s]", bindRuleAdapter.BindName))
			statusMap[resolveEntityName(bindRuleAdapter.BindName, customResourceName, customResourceNamespace, explicitName)] = fmt.Sprintf("error: %s", missingErr)
			retryErr = firstRetryableError(retryErr, missingErr)
			continue
		}
		bindRuleDemand := convertBindRuleAdapterToBindRule(bindRuleAdapter, customResourceName, customResourceNamespace, explicitName, methodType)
		var existingRules []*consulApi.ACLBindingRule
		existingRules, _, err = aclClient.BindingRuleList(applicableAuthMethod, &consulApi.QueryOptions{})
		if err != nil {
			return &statusMap, err
		}
		var existingDescription string
		for _, existing := range existingRules {
			if existing.BindName == bindRuleDemand.BindName {
				bindRuleDemand.ID = existing.ID
				existingDescription = existing.Description
				break
			}
		}
		bindRuleDemand.Description = withOwnerAdded(bindRuleDemand.Description, existingDescription, customResourceNamespace)
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
			retryErr = firstRetryableError(retryErr, err)
		} else {
			statusMap[fmt.Sprintf("Bind rule for %s with name %s",
				bindRuleDemand.BindType, bindRuleDemand.BindName)] = fmt.Sprintf("%sd", action)
		}
	}
	// Only transient errors are returned (the request is requeued), other errors were logged and recorded in the status
	return &statusMap, retryErr
}

// bindRuleAuthMethod returns the auth method of the binding rule: the per-rule override or the global one.
func bindRuleAuthMethod(bindRuleAdapter ACLBindingRuleAdapter) string {
	if bindRuleAdapter.AuthMethod != "" {
		return bindRuleAdapter.AuthMethod
	}
	return authMethod
}

// authMethodTypeResolver returns the type of a Consul auth method and whether the method exists,
// reading each method once.
func authMethodTypeResolver() func(name string) (string, bool, error) {
	type method struct {
		typ   string
		found bool
	}
	methods := map[string]method{}
	return func(name string) (string, bool, error) {
		if m, ok := methods[name]; ok {
			return m.typ, m.found, nil
		}
		am, _, err := aclClient.AuthMethodRead(name, &consulApi.QueryOptions{})
		if err != nil && !isErrNotFound(err) {
			return "", false, err
		}
		var m method
		if am != nil {
			m = method{typ: am.Type, found: true}
		}
		methods[name] = m
		return m.typ, m.found, nil
	}
}

// convertBindRuleAdapterToBindRule builds the Consul binding rule. authMethodType is the type of the
// auth method of the rule and defines the selector generated from ServiceAccountName.
func convertBindRuleAdapterToBindRule(bindRuleAdapter ACLBindingRuleAdapter, customResourceName string, customResourceNamespace string, explicitName bool, authMethodType string) consulApi.ACLBindingRule {
	bindingRule := consulApi.ACLBindingRule{}
	bindingRule.ID = bindRuleAdapter.ID
	if explicitName {
		bindingRule.BindName = bindRuleAdapter.BindName
	} else {
		bindingRule.BindName = convertEntityName(bindRuleAdapter.BindName, customResourceName, customResourceNamespace)
	}
	bindingRule.BindType = "role"
	bindingRule.AuthMethod = bindRuleAuthMethod(bindRuleAdapter)
	bindingRule.Description = bindRuleAdapter.Description
	if bindRuleAdapter.Selector != "" {
		// Explicit selector wins and is passed to Consul verbatim.
		bindingRule.Selector = bindRuleAdapter.Selector
	} else if bindRuleAdapter.ServiceAccountName != "" {
		if strings.Contains(bindRuleAdapter.ServiceAccountName, "${") {
			// Templated ServiceAccountName (e.g. "${value.serviceaccount}") describes a
			// dynamic/global rule that must match any login, not a concrete SA. Building a
			// "value.serviceaccount == \"${...}\"" selector would produce a dead literal that
			// matches nothing, so leave the selector empty (match all).
			log.Info(fmt.Sprintf("ServiceAccountName [%s] for BindName [%s] is a template, skipping selector generation (rule will match all logins)",
				bindRuleAdapter.ServiceAccountName, bindingRule.BindName))
		} else if authMethodType == "kubernetes" {
			// The kubernetes auth method exposes the service account as serviceaccount.* fields.
			bindingRule.Selector = fmt.Sprintf("serviceaccount.namespace == \"%s\" and serviceaccount.name == \"%s\"",
				customResourceNamespace,
				bindRuleAdapter.ServiceAccountName)
		} else {
			// The jwt auth method exposes the claims mapped by ClaimMappings as value.* fields.
			bindingRule.Selector = fmt.Sprintf("value.namespace == \"%s\" and value.serviceaccount == \"%s\"",
				customResourceNamespace,
				bindRuleAdapter.ServiceAccountName)
		}
	}
	return bindingRule
}

const defaultJWKSURL = "http://localhost:8080/openid/v1/jwks"

const openIDConfigPath = "/.well-known/openid-configuration"

// maxOpenIDConfigSize limits the OpenID configuration read from the JWKS proxy.
const maxOpenIDConfigSize = 1 << 20

var openIDConfigHTTPClient = &http.Client{Timeout: 10 * time.Second}

// openIDConfigURL returns the URL of the OpenID configuration served by the same host as jwksURL.
func openIDConfigURL(jwksURL string) (string, error) {
	u, err := url.Parse(jwksURL)
	if err != nil {
		return "", fmt.Errorf("invalid JWKS_URL %q: %w", jwksURL, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid JWKS_URL %q: scheme and host are required", jwksURL)
	}
	u.Path = openIDConfigPath
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

// detectIssuer reads the issuer of the service account tokens from the OpenID configuration
// served next to jwksURL (the JWKS proxy exposes the configuration of the API server).
func detectIssuer(jwksURL string) (string, error) {
	configURL, err := openIDConfigURL(jwksURL)
	if err != nil {
		return "", err
	}
	resp, err := openIDConfigHTTPClient.Get(configURL)
	if err != nil {
		return "", fmt.Errorf("error requesting OpenID configuration %q: %w", configURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("error requesting OpenID configuration %q: unexpected status %d", configURL, resp.StatusCode)
	}
	var openIDConfig struct {
		Issuer string `json:"issuer"`
	}
	// The configuration is a small document, a larger body is not read into memory.
	if err = json.NewDecoder(io.LimitReader(resp.Body, maxOpenIDConfigSize)).Decode(&openIDConfig); err != nil {
		return "", fmt.Errorf("error parsing OpenID configuration %q: %w", configURL, err)
	}
	if openIDConfig.Issuer == "" {
		return "", fmt.Errorf("OpenID configuration %q has no issuer", configURL)
	}
	return openIDConfig.Issuer, nil
}

// boundIssuerAndAudiences returns BoundIssuer and BoundAudiences of the auth method. Values set in
// BOUND_ISSUER and BOUND_AUDIENCES (comma-separated) take precedence. Otherwise the issuer is detected
// from the OpenID configuration and the audiences default to the issuer (the default --api-audiences).
func boundIssuerAndAudiences(jwksURL string) (string, []string, error) {
	issuer := strings.TrimSpace(os.Getenv("BOUND_ISSUER"))
	audiences := splitList(os.Getenv("BOUND_AUDIENCES"))
	if issuer == "" {
		detected, err := detectIssuer(jwksURL)
		if err != nil {
			return "", nil, err
		}
		issuer = detected
	}
	if len(audiences) == 0 {
		audiences = []string{issuer}
	}
	return issuer, audiences, nil
}

// authMethodUpToDate reports whether the existing auth method already has the desired type,
// description and config values. Config keys that are not set by the operator are ignored.
func authMethodUpToDate(existing, desired *consulApi.ACLAuthMethod) bool {
	if existing.Type != desired.Type || existing.Description != desired.Description {
		return false
	}
	for key, value := range desired.Config {
		want, err := json.Marshal(value)
		if err != nil {
			return false
		}
		got, err := json.Marshal(existing.Config[key])
		if err != nil || !bytes.Equal(want, got) {
			return false
		}
	}
	return true
}

// EnsureApplicationsAuthMethod creates or updates the applications-k8s-m2m auth method in Consul.
func EnsureApplicationsAuthMethod() error {
	const amName = "applications-k8s-m2m"
	existing, _, err := aclClient.AuthMethodRead(amName, &consulApi.QueryOptions{})
	if err != nil {
		return fmt.Errorf("error reading auth method %q: %w", amName, err)
	}

	jwksURL := os.Getenv("JWKS_URL")
	if jwksURL == "" {
		jwksURL = defaultJWKSURL
	}
	issuer, audiences, err := boundIssuerAndAudiences(jwksURL)
	if err != nil {
		return fmt.Errorf("error resolving BoundIssuer of auth method %q: %w", amName, err)
	}

	am := &consulApi.ACLAuthMethod{
		Name:        amName,
		Type:        "jwt",
		Description: "Auth method for application M2M authentication",
		Config: map[string]interface{}{
			"JWKSURL":        jwksURL,
			"BoundIssuer":    issuer,
			"BoundAudiences": audiences,
			"ClaimMappings": map[string]string{
				"/kubernetes.io/namespace":           "namespace",
				"/kubernetes.io/serviceaccount/name": "serviceaccount",
			},
		},
	}
	if existing == nil {
		_, _, err = aclClient.AuthMethodCreate(am, &consulApi.WriteOptions{})
		if err != nil {
			return fmt.Errorf("error creating auth method %q: %w", amName, err)
		}
		log.Info(fmt.Sprintf("Auth method %q created", amName), "boundIssuer", issuer, "boundAudiences", audiences)
	} else if authMethodUpToDate(existing, am) {
		log.Info(fmt.Sprintf("Auth method %q is up to date", amName))
	} else {
		_, _, err = aclClient.AuthMethodUpdate(am, &consulApi.WriteOptions{})
		if err != nil {
			return fmt.Errorf("error updating auth method %q: %w", amName, err)
		}
		log.Info(fmt.Sprintf("Auth method %q updated", amName), "boundIssuer", issuer, "boundAudiences", audiences)
	}
	return nil
}

// EnsureApplicationsAuthMethodWithRetry calls EnsureApplicationsAuthMethod in a loop with
// exponential backoff until it succeeds or ctx is cancelled. Intended to run as a goroutine.
func EnsureApplicationsAuthMethodWithRetry(ctx context.Context) {
	backoff := time.Second
	for {
		if err := EnsureApplicationsAuthMethod(); err != nil {
			log.Error(err, "failed to ensure applications-k8s-m2m auth method, retrying", "backoff", backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 5*time.Minute {
				backoff *= 2
			}
			continue
		}
		log.Info("Auth method applications-k8s-m2m ensured successfully")
		return
	}
}

// readRole returns the role or nil when it does not exist. Any other error is returned, so that a
// failed read (for example no cluster leader) is not taken for an absent role.
func readRole(roleName string) (*consulApi.ACLRole, error) {
	role, _, err := aclClient.RoleReadByName(roleName, &consulApi.QueryOptions{})
	if err != nil && !isErrNotFound(err) {
		return nil, err
	}
	if role == nil || err != nil {
		log.Info(fmt.Sprintf("There is no role with name %s", roleName))
		return nil, nil
	}
	return role, nil
}

// readPolicy returns the policy or nil when it does not exist. Any other error is returned, so that a
// failed read (for example no cluster leader) is not taken for an absent policy.
func readPolicy(policyName string) (*consulApi.ACLPolicy, error) {
	policy, _, err := aclClient.PolicyReadByName(policyName, &consulApi.QueryOptions{})
	if err != nil && !isErrNotFound(err) {
		return nil, err
	}
	if policy == nil || err != nil {
		log.Info(fmt.Sprintf("There is no policy with name %s", policyName))
		return nil, nil
	}
	return policy, nil
}

func isErrNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), errNotFound)
}
