package service

import (
	"github.com/epinio/epinio/helpers/kubernetes"
	"github.com/epinio/epinio/internal/api/v1/response"
	"github.com/epinio/epinio/internal/application"
	"github.com/epinio/epinio/internal/namespaces"
	"github.com/epinio/epinio/internal/services"
	apierror "github.com/epinio/epinio/pkg/api/core/v1/errors"
	"github.com/epinio/epinio/pkg/api/core/v1/models"
	"github.com/gin-gonic/gin"
)

// Update handles the API endpoint PATCH /namespaces/:namespace/applications/:app
// It modifies the specified application. Currently this is only the
// number of instances to run.
func (sc Controller) Update(c *gin.Context) apierror.APIErrors { // nolint:gocyclo // simplification defered
	ctx := c.Request.Context()
	namespace := c.Param("namespace")
	serviceName := c.Param("service")

	cluster, err := kubernetes.GetCluster(ctx)
	if err != nil {
		return apierror.InternalError(err)
	}

	exists, err := namespaces.Exists(ctx, cluster, namespace)
	if err != nil {
		return apierror.InternalError(err)
	}

	if !exists {
		return apierror.NamespaceIsNotKnown(namespace)
	}

	service, err := services.Lookup(ctx, cluster, namespace, serviceName)
	if err != nil {
		if err.Error() == "service not found" {
			return apierror.ServiceIsNotKnown(serviceName)
		}
		if err != nil {
			return apierror.InternalError(err)
		}
	}

	// Retrieve and validate update request ...

	var updateRequest models.ServiceUpdateRequest
	err = c.BindJSON(&updateRequest)
	if err != nil {
		return apierror.BadRequest(err)
	}

	// Save changes to resource

	err = services.UpdateService(ctx, cluster, service, updateRequest)
	if err != nil {
		return apierror.InternalError(err)
	}

	// Determine bound apps, as candidates for restart.

	appNames, err := application.BoundAppsNamesFor(ctx, cluster, namespace, serviceName)
	if err != nil {
		return apierror.InternalError(err)
	}

	// Perform restart on the candidates which are actually running

	for _, appName := range appNames {
		app, err := application.Lookup(ctx, cluster, namespace, appName)
		if err != nil {
			return apierror.InternalError(err)
		}

		// Restart workload, if any
		if app.Workload != nil {
			// TODO :: This plain restart is different from all other restarts
			// (scaling, ev change, bound services change) ... The deployment
			// actually does not change, at all. A resource the deployment
			// references/uses changed, i.e. the service. We still have to
			// trigger the restart somehow, so that the pod mounting the
			// service remounts it for the new/changed keys.

			err = application.NewWorkload(cluster, app.Meta).Restart(ctx)
			if err != nil {
				return apierror.InternalError(err)
			}
		}
	}

	// Done

	response.OK(c)
	return nil
}