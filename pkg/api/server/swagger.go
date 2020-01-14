package server

import (
	"github.com/containers/libpod/pkg/api/handlers"
	"github.com/containers/libpod/pkg/api/handlers/utils"
)

// No such image
// swagger:response NoSuchImage
type swagErrNoSuchImage struct {
	// in:body
	Body struct {
		utils.ErrorModel
	}
}

// No such container
// swagger:response NoSuchContainer
type swagErrNoSuchContainer struct {
	// in:body
	Body struct {
		utils.ErrorModel
	}
}

// No such volume
// swagger:response NoSuchVolume
type swagErrNoSuchVolume struct {
	// in:body
	Body struct {
		utils.ErrorModel
	}
}

// No such pod
// swagger:response NoSuchPod
type swagErrNoSuchPod struct {
	// in:body
	Body struct {
		utils.ErrorModel
	}
}

// No such network
// swagger:response NoSuchNetwork
type swagErrNoSuchNetwork struct {
	// in:body
	Body struct {
		utils.ErrorModel
	}
}

// Internal error
// swagger:response InternalError
type swagInternalError struct {
	// in:body
	Body struct {
		utils.ErrorModel
	}
}

// Generic error
// swagger:response GenericError
type swagGenericError struct {
	// in:body
	Body struct {
		utils.ErrorModel
	}
}

// Conflict error
// swagger:response ConflictError
type swagConflictError struct {
	// in:body
	Body struct {
		utils.ErrorModel
	}
}

// Bad parameter
// swagger:response BadParamError
type swagBadParamError struct {
	// in:body
	Body struct {
		utils.ErrorModel
	}
}

// Container already started
// swagger:response ContainerAlreadyStartedError
type swagContainerAlreadyStartedError struct {
	// in:body
	Body struct {
		utils.ErrorModel
	}
}

// Container already stopped
// swagger:response ContainerAlreadyStoppedError
type swagContainerAlreadyStopped struct {
	// in:body
	Body struct {
		utils.ErrorModel
	}
}

// Pod already started
// swagger:response PodAlreadyStartedError
type swagPodAlreadyStartedError struct {
	// in:body
	Body struct {
		utils.ErrorModel
	}
}

// Pod already stopped
// swagger:response PodAlreadyStoppedError
type swagPodAlreadyStopped struct {
	// in:body
	Body struct {
		utils.ErrorModel
	}
}

// Image summary
// swagger:response DockerImageSummary
type swagImageSummary struct {
	// in:body
	Body struct {
		handlers.ImageSummary
	}
}

// List Containers
// swagger:response DocsListContainer
type swagListContainers struct {
	// in:body
	Body struct {
		// This causes go-swagger to crash
		//handlers.Container
	}
}

// Prune response
// swagger:response ContainerPruneResponse
type swagContainerPruneResponse struct {
	// in:body
	Body struct {
		handlers.ContainerPruneResponse
	}
}

// To be determined
// swagger:response tbd
type swagTBD struct {
}
