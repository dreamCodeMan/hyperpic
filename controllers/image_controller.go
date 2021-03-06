// Copyright 2017 Axel Etcheverry. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package controllers

import (
	"crypto/md5"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/euskadi31/go-server"
	"github.com/hyperscale/hyperpic/config"
	"github.com/hyperscale/hyperpic/httputil"
	"github.com/hyperscale/hyperpic/image"
	"github.com/hyperscale/hyperpic/metrics"
	"github.com/hyperscale/hyperpic/middlewares"
	"github.com/hyperscale/hyperpic/provider"
	"github.com/justinas/alice"
	"github.com/rs/zerolog/log"
	"gopkg.in/h2non/filetype.v1"
)

// ImageController struct
type ImageController struct {
	cfg            *config.Configuration
	optionParser   *image.OptionParser
	sourceProvider provider.SourceProvider
	cacheProvider  provider.CacheProvider
}

// NewImageController func
func NewImageController(
	cfg *config.Configuration,
	optionParser *image.OptionParser,
	sourceProvider provider.SourceProvider,
	cacheProvider provider.CacheProvider,
) (*ImageController, error) {
	return &ImageController{
		cfg:            cfg,
		optionParser:   optionParser,
		sourceProvider: sourceProvider,
		cacheProvider:  cacheProvider,
	}, nil
}

// Mount endpoints
func (c ImageController) Mount(r *server.Router) {
	chain := alice.New(
		middlewares.NewPathHandler(),
		middlewares.NewImageExtensionFilterHandler(c.cfg),
	)

	public := chain.Append(
		middlewares.NewOptionsHandler(c.optionParser),
		middlewares.NewContentTypeHandler(),
		middlewares.NewClientHintsHandler(),
	)

	private := chain.Append(
		middlewares.NewAuthHandler(c.cfg.Auth),
	)

	r.AddPrefixRoute("/", public.ThenFunc(c.GetHandler)).Methods(http.MethodGet)
	r.AddPrefixRoute("/", private.ThenFunc(c.PostHandler)).Methods(http.MethodPost)
	r.AddPrefixRoute("/", private.ThenFunc(c.DeleteHandler)).Methods(http.MethodDelete)
}

// GetHandler endpoint
func (c ImageController) GetHandler(w http.ResponseWriter, r *http.Request) {
	options, err := middlewares.OptionsFromContext(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("Error while parsing options")

		http.Error(w, "Error while parsing options", http.StatusBadRequest)

		return
	}
	// xlog.Infof("options: %#v", options)

	resource := &image.Resource{
		Path:    r.URL.Path,
		Options: options,
	}

	// w.Header().Set("Link", `</worker/client-hints.js>; rel="serviceworker"`)

	// fetch from cache
	if resource, err := c.cacheProvider.Get(resource); err == nil {
		w.Header().Set("X-Image-From", "cache")

		httputil.ServeImage(w, r, resource)

		metrics.CacheHit.With(map[string]string{}).Add(1)
		metrics.ImageDeliveredBytes.With(map[string]string{}).Add(float64(len(resource.Body)))

		return
	}

	resource, err = c.sourceProvider.Get(resource)
	if err != nil {
		if os.IsNotExist(err) {
			msg := fmt.Sprintf("File %s not found", r.URL.Path)

			log.Info().Msg(msg)

			http.Error(w, msg, http.StatusNotFound)

			return
		}

		log.Error().Err(err).Msg("Source Provider")

		http.Error(w, err.Error(), http.StatusNotFound)

		return
	}

	if err := image.ProcessImage(resource); err != nil {
		log.Error().Err(err).Msg("Error while processing the image")

		http.Error(w, "Error while processing the image", http.StatusInternalServerError)

		return
	}

	w.Header().Set("X-Image-From", "source")

	httputil.ServeImage(w, r, resource)

	// save resource in cache
	go func(r *image.Resource) {
		if err := c.cacheProvider.Set(r); err != nil {
			log.Error().Err(err).Msg("Cache Provider")
		}
	}(resource)

	metrics.CacheMiss.With(map[string]string{}).Add(1)
	metrics.ImageDeliveredBytes.With(map[string]string{}).Add(float64(len(resource.Body)))
}

// PostHandler endpoint
func (c ImageController) PostHandler(w http.ResponseWriter, r *http.Request) {
	resource := &image.Resource{
		Path: r.URL.Path,
	}

	r.ParseMultipartForm(32 << 20)

	file, _, err := r.FormFile("image")
	if err != nil {
		server.FailureFromError(w, http.StatusBadRequest, err)

		return
	}

	defer file.Close()

	body, err := ioutil.ReadAll(file)
	if err != nil {
		server.FailureFromError(w, http.StatusBadRequest, err)

		return
	}

	resource.Body = body

	if err := c.sourceProvider.Set(resource); err != nil {
		server.FailureFromError(w, http.StatusInternalServerError, err)

		return
	}

	// delete cache from source file
	go c.cacheProvider.Del(resource)

	mimeType := http.DetectContentType(body)

	// If cannot infer the type, infer it via magic numbers
	if mimeType == "application/octet-stream" {
		kind, err := filetype.Get(body)
		if err == nil && kind.MIME.Value != "" {
			mimeType = kind.MIME.Value
		}
	}

	h := md5.New()
	h.Write(body)

	lenght := len(body)

	server.JSON(w, http.StatusCreated, map[string]interface{}{
		"file": r.URL.Path,
		"size": lenght,
		"type": mimeType,
		"hash": fmt.Sprintf("%x", h.Sum(nil)),
	})

	metrics.ImageReceivedBytes.With(map[string]string{}).Add(float64(lenght))
}

// DeleteHandler endpoint
func (c ImageController) DeleteHandler(w http.ResponseWriter, r *http.Request) {
	resource := &image.Resource{
		Path: r.URL.Path,
	}

	response := map[string]bool{
		"cache":  false,
		"source": false,
	}

	if from := r.URL.Query().Get("from"); from != "" {
		switch from {
		case "source":
			response["cache"] = (c.cacheProvider.Del(resource) == nil)
			response["source"] = (c.sourceProvider.Del(resource) == nil)
		default:
			response["cache"] = (c.cacheProvider.Del(resource) == nil)
		}
	}

	server.JSON(w, http.StatusOK, resource)
}
