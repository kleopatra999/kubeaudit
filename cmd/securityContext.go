package cmd

import (
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	v1beta1 "k8s.io/api/apps/v1beta1"
	apiv1 "k8s.io/api/core/v1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	"reflect"
)

func checkSecurityContext(container apiv1.Container, result *Result) {
	result.capsDropped = true

	if container.SecurityContext == nil {
		result.err = 1
		return
	}

	if container.SecurityContext.Capabilities == nil {
		result.err = 2
		return
	}

	if container.SecurityContext.Capabilities.Add != nil {
		result.err = 3
		result.capsAdded = container.SecurityContext.Capabilities.Add
	}

	if container.SecurityContext.Capabilities.Drop == nil {
		result.err = 3
		result.capsDropped = false
	}

	return
}

func printResultSC(results []Result) {
	for _, result := range results {
		switch err := result.err; err {
		case 1:
			log.WithField("type", result.kubeType).Error(result.namespace,
				"/", result.name)
		case 2:
			log.WithFields(log.Fields{
				"type": result.kubeType,
			}).Warn("Capabilities field not defined! ", result.namespace, "/", result.name)
		case 3:
			if result.capsAdded != nil {
				log.WithFields(log.Fields{
					"type": result.kubeType,
					"caps": result.capsAdded}).
					Warn("Capabilities added to ", result.namespace, "/", result.name)
			}

			if !result.capsDropped {
				log.WithField("type", result.kubeType).
					Warn("No capabilities were dropped! ", result.namespace, "/", result.name)
			}
		}

	}
}

func auditSecurityContext(items Items) (results []Result) {
	for _, item := range items.Iter() {
		containers, result := containerIter(item)
		for _, container := range containers {
			checkSecurityContext(container, result)
			if result != nil && result.err > 0 {
				results = append(results, *result)
				break
			}

		}
	}

	printResultSC(results)
	defer wg.Done()
	return
}

var securityContextCmd = &cobra.Command{
	Use:   "sc",
	Short: "Audit container security contexts",
	Long: `This command determines which containers in a kubernetes cluster
are running as root.
An INFO log is given when a container has a securityContext
An ERROR log is generated when a container does not have a defined securityContext
A WARN log is generated when some linux capabilities are added or not dropped
This command is also a root command, check kubeaudit sc --help
Example usage:
kubeaudit sc
kubeaudit sc nonroot
kubeaudit sc rootfs`,
	Run: func(cmd *cobra.Command, args []string) {
		if rootConfig.json {
			log.SetFormatter(&log.JSONFormatter{})
		}

		if rootConfig.config != "" {
			wg.Add(1)
			resource := getKubeResource(rootConfig.config)
			log.Info(reflect.TypeOf(resource).String())
			switch reflect.TypeOf(resource).String() {
			case "*v1beta1.Deployment":
				auditSecurityContext(kubeAuditDeployments{list: convertDeploymentToDeploymentList(*resource.(*v1beta1.Deployment))})
			case "*v1beta1.StatefulSet":
				auditSecurityContext(kubeAuditStatefulSets{list: convertStatefulSetToStatefulSetList(*resource.(*v1beta1.StatefulSet))})
			case "*v1beta1.DaemonSet":
				auditSecurityContext(kubeAuditDaemonSets{list: convertDaemonSetToDaemonSetList(*resource.(*extensionsv1beta1.DaemonSet))})
			case "*v1.Pod":
				auditSecurityContext(kubeAuditPods{list: convertPodToPodList(*resource.(*apiv1.Pod))})
			case "*v1.ReplicationController":
				auditSecurityContext(kubeAuditReplicationControllers{list: convertReplicationControllerToReplicationList(*resource.(*apiv1.ReplicationController))})
			}
		} else {
			kube, err := kubeClient(rootConfig.kubeConfig)
			if err != nil {
				log.Error(err)
			}

			// fetch deployments, statefulsets, daemonsets
			// and pods which do not belong to another abstraction
			deployments := getDeployments(kube)
			statefulSets := getStatefulSets(kube)
			daemonSets := getDaemonSets(kube)
			pods := getPods(kube)
			replicationControllers := getReplicationControllers(kube)

			wg.Add(5)
			go auditSecurityContext(kubeAuditStatefulSets{list: statefulSets})
			go auditSecurityContext(kubeAuditDaemonSets{list: daemonSets})
			go auditSecurityContext(kubeAuditPods{list: pods})
			go auditSecurityContext(kubeAuditReplicationControllers{list: replicationControllers})
			go auditSecurityContext(kubeAuditDeployments{list: deployments})
			wg.Wait()
		}
	},
}

func init() {
	RootCmd.AddCommand(securityContextCmd)
}
