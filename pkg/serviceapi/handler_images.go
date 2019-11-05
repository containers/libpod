package serviceapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/containers/libpod/libpod"
	image2 "github.com/containers/libpod/libpod/image"
	"github.com/containers/libpod/pkg/util"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

func registerImagesHandlers(r *mux.Router) error {
	r.Handle(unversionedPath("/images/json"), serviceHandler(getImages)).Methods("GET")
	r.Handle(unversionedPath("/images/{name:..*}/json"), serviceHandler(image))
	r.Handle(unversionedPath("/images/{name:..*}/tag"), serviceHandler(tagImage)).Methods("POST")
	r.Handle(unversionedPath("/images/create"), serviceHandler(createImage)).Methods("POST")
	return nil
}

func createImage(w http.ResponseWriter, r *http.Request, runtime *libpod.Runtime) {
	/*
	   fromImage – Name of the image to pull. The name may include a tag or digest. This parameter may only be used when pulling an image. The pull is cancelled if the HTTP connection is closed.
	   fromSrc – Source to import. The value may be a URL from which the image can be retrieved or - to read the image from the request body. This parameter may only be used when importing an image.
	   repo – Repository name given to an image when it is imported. The repo may include a tag. This parameter may only be used when importing an image.
	   tag – Tag or digest. If empty when pulling an image, this causes all tags for the given image to be pulled.
	*/
	ctx := context.Background()
	fromImage := r.Form.Get("fromImage")
	// TODO
	// We are eating the output right now because we haven't talked about how to deal with multiple responses yet
	_, err := runtime.ImageRuntime().New(ctx, fromImage, "", "", nil, &image2.DockerRegistryOptions{}, image2.SigningOptions{}, nil, util.PullImageAlways)
	if err != nil {
		Error(w, "Something went wrong.", http.StatusInternalServerError, err)
		return
	}
	// Success
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "")
	return
}

func tagImage(w http.ResponseWriter, r *http.Request, runtime *libpod.Runtime) {
	// /v1.xx/images/(name)/tag
	name := mux.Vars(r)["name"]
	newImage, err := runtime.ImageRuntime().NewFromLocal(name)
	if err != nil {
		Error(w, "Something went wrong.", http.StatusNotFound, err)
		return
	}
	tag := "latest"
	if len(r.Form.Get("tag")) > 0 {
		tag = r.Form.Get("tag")
	}
	if len(r.Form.Get("repo")) < 1 {
		Error(w, "Something went wrong.", http.StatusBadRequest, errors.New("repo parameter is required to tag an image"))
		return
	}
	repo := r.Form.Get("repo")
	tagName := fmt.Sprintf("%s:%s", repo, tag)
	if err := newImage.TagImage(tagName); err != nil {
		Error(w, "Something went wrong.", http.StatusInternalServerError, err)
		return
	}
	// Success is a 201?
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintln(w, "")
}

func image(w http.ResponseWriter, r *http.Request, runtime *libpod.Runtime) {
	name := mux.Vars(r)["name"]
	newImage, err := runtime.ImageRuntime().NewFromLocal(name)
	if err != nil {
		Error(w, "Something went wrong.", http.StatusNotFound, errors.Wrapf(err, "Failed to find image %s", name))
		return
	}
	ctx := context.Background()

	switch r.Method {
	case "DELETE":
		force := false
		if len(r.Form.Get("force")) > 0 {
			force, err = strconv.ParseBool(r.Form.Get("force"))
			if err != nil {
				Error(w, "Something went wrong.", http.StatusBadRequest, err)
				return
			}
		}
		r, err := runtime.RemoveImage(ctx, newImage, force)
		if err != nil {
			Error(w, "Something went wrong.", http.StatusInternalServerError, err)
			return
		}
		// TODO
		// This will need to be fixed for proper response, like Deleted: and Untagged:
		buffer, _ := json.Marshal(r)
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, string(buffer))
		return
	}

	info, err := newImage.Inspect(context.Background())
	if err != nil {
		Error(w, "Server error", http.StatusInternalServerError, errors.Wrapf(err, "Failed to inspect image %s", name))
		return
	}

	inspect, err := ImageDataToImageInspect(info)
	buffer, err := json.Marshal(inspect)
	if err != nil {
		Error(w, "Server error", http.StatusInternalServerError, errors.Wrapf(err,"Failed to convert API ImageInspect '%s' to json", inspect.ID))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, string(buffer))
}

func getImages(w http.ResponseWriter, r *http.Request, runtime *libpod.Runtime) {
	query := struct {
		all     bool
		filters string
		digests bool
	}{
		// This is where you can override the golang default value for one of fields
	}

	var err error
	t := r.Form.Get("all")
	if t != "" {
		query.all, err = strconv.ParseBool(t)
		if err != nil {
			Error(w, "Server error", http.StatusBadRequest, errors.Wrapf(err, "Failed to parse 'all' parameter %s", t))
			return
		}
	}

	// TODO How do we want to process filters
	t = r.Form.Get("filters")
	if t != "" {
		query.filters = t
	}

	t = r.Form.Get("digests")
	if t != "" {
		query.digests, err = strconv.ParseBool(t)
		if err != nil {
			Error(w, "Server error", http.StatusBadRequest, errors.Wrapf(err, "Failed to parse 'digests' parameter %s", t))
			return
		}
	}

	// FIXME placeholder until filters are implemented
	_ = query

	images, err := runtime.ImageRuntime().GetImages()
	if err != nil {
		Error(w, "Something went wrong.", http.StatusInternalServerError, errors.Wrapf(err, "Failed to obtain the list of images from storage"))
		return
	}

	var summaries []*ImageSummary
	for _, img := range images {
		i, err := ImageToImageSummary(img)
		if err != nil {
			Error(w, "Something went wrong.", http.StatusInternalServerError, errors.Wrapf(err, "Failed to convert storage image '%s' to API image"))
			return
		}
		summaries = append(summaries, i)
	}

	buffer, err := json.Marshal(summaries)
	if err != nil {
		Error(w, "Something went wrong.", http.StatusInternalServerError, errors.Wrapf(err, "Failed to marshal API image(s) '%s' to json"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, string(buffer))
}
