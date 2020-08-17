package compat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/containers/podman/v2/libpod"
	"github.com/containers/podman/v2/libpod/events"
	"github.com/containers/podman/v2/pkg/api/handlers/utils"
	"github.com/containers/podman/v2/pkg/domain/entities"
	"github.com/gorilla/schema"
	jsoniter "github.com/json-iterator/go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// filtersFromRequests extracts the "filters" parameter from the specified
// http.Request.  The paramater can either be a `map[string][]string` as done
// in new versions of Docker and libpod, or a `map[string]map[string]bool` as
// done in older versions of Docker.  We have to do a bit of Yoga to support
// both - just as Docker does as well.
//
// Please refer to https://github.com/containers/podman/issues/6899 for some
// background.
func filtersFromRequest(r *http.Request) ([]string, error) {
	var (
		compatFilters map[string]map[string]bool
		filters       map[string][]string
		libpodFilters []string
		raw           []byte
	)

	if _, found := r.URL.Query()["filters"]; found {
		raw = []byte(r.Form.Get("filters"))
	} else {
		return []string{}, nil
	}

	// Backwards compat with older versions of Docker.
	if err := json.Unmarshal(raw, &compatFilters); err == nil {
		for filterKey, filterMap := range compatFilters {
			for filterValue, toAdd := range filterMap {
				if toAdd {
					libpodFilters = append(libpodFilters, fmt.Sprintf("%s=%s", filterKey, filterValue))
				}
			}
		}
		return libpodFilters, nil
	}

	if err := json.Unmarshal(raw, &filters); err != nil {
		return nil, err
	}

	for filterKey, filterSlice := range filters {
		for _, filterValue := range filterSlice {
			libpodFilters = append(libpodFilters, fmt.Sprintf("%s=%s", filterKey, filterValue))
		}
	}

	return libpodFilters, nil
}

// NOTE: this endpoint serves both the docker-compatible one and the new libpod
// one.
func GetEvents(w http.ResponseWriter, r *http.Request) {
	var (
		fromStart bool
		decoder   = r.Context().Value("decoder").(*schema.Decoder)
		runtime   = r.Context().Value("runtime").(*libpod.Runtime)
		json      = jsoniter.ConfigCompatibleWithStandardLibrary // FIXME: this should happen on the package level
	)

	// NOTE: the "filters" parameter is extracted separately for backwards
	// compat via `fitlerFromRequest()`.
	query := struct {
		Since  string `schema:"since"`
		Until  string `schema:"until"`
		Stream bool   `schema:"stream"`
	}{
		Stream: true,
	}
	if err := decoder.Decode(&query, r.URL.Query()); err != nil {
		utils.Error(w, "Failed to parse parameters", http.StatusBadRequest, errors.Wrapf(err, "Failed to parse parameters for %s", r.URL.String()))
		return
	}

	if len(query.Since) > 0 || len(query.Until) > 0 {
		fromStart = true
	}

	libpodFilters, err := filtersFromRequest(r)
	if err != nil {
		utils.Error(w, "Failed to parse parameters", http.StatusBadRequest, errors.Wrapf(err, "Failed to parse parameters for %s", r.URL.String()))
		return
	}

	eventChannel := make(chan *events.Event)
	errorChannel := make(chan error)

	// Start reading events.
	go func() {
		readOpts := events.ReadOptions{
			FromStart:    fromStart,
			Stream:       query.Stream,
			Filters:      libpodFilters,
			EventChannel: eventChannel,
			Since:        query.Since,
			Until:        query.Until,
		}
		errorChannel <- runtime.Events(r.Context(), readOpts)
	}()

	var coder *jsoniter.Encoder
	var writeHeader sync.Once

	for stream := true; stream; stream = query.Stream {
		select {
		case err := <-errorChannel:
			if err != nil {
				utils.InternalServerError(w, err)
				return
			}
		case evt := <-eventChannel:
			writeHeader.Do(func() {
				// Use a sync.Once so that we write the header
				// only once.
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
				coder = json.NewEncoder(w)
				coder.SetEscapeHTML(true)
			})

			if evt == nil {
				continue
			}

			e := entities.ConvertToEntitiesEvent(*evt)
			if err := coder.Encode(e); err != nil {
				logrus.Errorf("unable to write json: %q", err)
			}
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}

	}
}
