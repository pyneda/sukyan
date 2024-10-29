package discovery

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
)

var KubernetesPaths = []string{
	"api/v1/pods",
	"api/v1/nodes",
	"api/v1/namespaces",
	"api/v1/services",
	"api/v1/secrets",
	"api/v1/configmaps",
	"api/v1/componentstatuses",
	"api/v1/persistentvolumes",
	"api/v1/persistentvolumeclaims",
	"api/v1/serviceaccounts",
	"apis/apps/v1/deployments",
	"apis/apps/v1/daemonsets",
	"apis/apps/v1/statefulsets",
	"apis/batch/v1/jobs",
	"apis/batch/v1/cronjobs",
	"apis/networking.k8s.io/v1/ingresses",
	"apis/rbac.authorization.k8s.io/v1/roles",
	"apis/rbac.authorization.k8s.io/v1/clusterroles",
	"apis/rbac.authorization.k8s.io/v1/rolebindings",
	"apis/storage.k8s.io/v1/storageclasses",
	"apis/certificates.k8s.io/v1/certificatesigningrequests",
	"apis/authentication.k8s.io/v1/tokenreviews",
	"apis/authorization.k8s.io/v1/subjectaccessreviews",
	"apis/autoscaling/v2/horizontalpodautoscalers",
	"kube-system/pods",
	"kube-public/configmaps",
}

func IsKubernetesValidationFunc(history *db.History) (bool, string, int) {
	bodyStr := string(history.ResponseBody)
	details := fmt.Sprintf("Kubernetes endpoint found: %s\n", history.URL)
	confidence := 0

	// Check for standard k8s error responses
	k8sErrors := map[string]string{
		"Unauthorized":                "missing authentication credentials",
		"Forbidden":                   "invalid credentials or insufficient permissions",
		"users.authentication.k8s.io": "authentication API",
		"forbidden: User":             "RBAC restriction",
	}

	if history.StatusCode == 401 || history.StatusCode == 403 {
		for errorStr, description := range k8sErrors {
			if strings.Contains(bodyStr, errorStr) {
				confidence = 90
				details += fmt.Sprintf("- Kubernetes API confirmed (%s)\n", description)
				details += "- Access is restricted, manual verification recommended\n"
				return true, details, confidence
			}
		}
	}

	if history.StatusCode == 200 {
		confidence = 80

		// Check response structure
		k8sIndicators := map[string]string{
			"kind":       "Kubernetes resource type indicator",
			"apiVersion": "API version field",
			"metadata":   "Resource metadata",
			"namespace":  "Namespace information",
			"status":     "Resource status field",
			"spec":       "Resource specification",
		}

		for indicator, description := range k8sIndicators {
			if strings.Contains(bodyStr, indicator) {
				confidence += 5
				details += fmt.Sprintf("- Contains %s\n", description)
			}
		}

		if strings.Contains(history.ResponseContentType, "application/json") {
			confidence += 5
		}
	}

	// Return true if we have any level of confidence
	if confidence > 0 {
		if history.StatusCode == 200 {
			details += "\nWARNING: Unauthenticated access to Kubernetes API detected\n"
		}
		return true, details, confidence
	}

	return false, "", 0
}

func DiscoverKubernetesEndpoints(baseURL string, opts http_utils.HistoryCreationOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         baseURL,
			Method:      "GET",
			Paths:       KubernetesPaths,
			Concurrency: 10,
			Timeout:     5,
			Headers: map[string]string{
				"Accept": "application/json",
			},
			HistoryCreationOptions: opts,
		},
		ValidationFunc: IsKubernetesValidationFunc,
		IssueCode:      db.KubernetesApiDetectedCode,
	})
}