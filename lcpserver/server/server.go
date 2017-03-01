// Copyright (c) 2016 Readium Foundation
//
// Redistribution and use in source and binary forms, with or without modification,
// are permitted provided that the following conditions are met:
//
// 1. Redistributions of source code must retain the above copyright notice, this
//    list of conditions and the following disclaimer.
// 2. Redistributions in binary form must reproduce the above copyright notice,
//    this list of conditions and the following disclaimer in the documentation and/or
//    other materials provided with the distribution.
// 3. Neither the name of the organization nor the names of its contributors may be
//    used to endorse or promote products derived from this software without specific
//    prior written permission
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
// ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
// WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
// DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT OWNER OR CONTRIBUTORS BE LIABLE FOR
// ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES
// (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES;
// LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND
// ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS
// SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package lcpserver

import (
	"crypto/tls"
	"net/http"
	"time"

	"github.com/abbot/go-http-auth"
	"github.com/gorilla/mux"

	"github.com/readium/readium-lcp-server/api"
	"github.com/readium/readium-lcp-server/index"
	"github.com/readium/readium-lcp-server/lcpserver/api"
	"github.com/readium/readium-lcp-server/license"
	"github.com/readium/readium-lcp-server/pack"
	"github.com/readium/readium-lcp-server/storage"
)

type Server struct {
	http.Server
	readonly bool
	idx      *index.Index
	st       *storage.Store
	lst      *license.Store
	cert     *tls.Certificate
	source   pack.ManualSource
}

func (s *Server) Store() storage.Store {
	return *s.st
}

func (s *Server) Index() index.Index {
	return *s.idx
}

func (s *Server) Licenses() license.Store {
	return *s.lst
}

func (s *Server) Certificate() *tls.Certificate {
	return s.cert
}

func (s *Server) Source() *pack.ManualSource {
	return &s.source
}

func New(bindAddr string, static string, readonly bool, idx *index.Index, st *storage.Store, lst *license.Store, cert *tls.Certificate, packager *pack.Packager, basicAuth *auth.BasicAuth) *Server {

	sr := api.CreateServerRouter(static)

	s := &Server{
		Server: http.Server{
			Handler:        sr.N,
			Addr:           bindAddr,
			WriteTimeout:   15 * time.Second,
			ReadTimeout:    15 * time.Second,
			MaxHeaderBytes: 1 << 20,
		},
		readonly: readonly,
		idx:      idx,
		st:       st,
		lst:      lst,
		cert:     cert,
		source:   pack.ManualSource{},
	}

	// Route.PathPrefix: http://www.gorillatoolkit.org/pkg/mux#Route.PathPrefix
	// Route.Subrouter: http://www.gorillatoolkit.org/pkg/mux#Route.Subrouter
	// Router.StrictSlash: http://www.gorillatoolkit.org/pkg/mux#Router.StrictSlash

	contentRoutesPathPrefix := "/contents"
	contentRoutes := sr.R.PathPrefix(contentRoutesPathPrefix).Subrouter().StrictSlash(false)

	s.handleFunc(sr.R, contentRoutesPathPrefix, apilcp.ListContents).Methods("GET")

	s.handleFunc(contentRoutes, "/{key}", apilcp.GetContent).Methods("GET")
	s.handlePrivateFunc(contentRoutes, "/{key}/licenses", apilcp.ListLicensesForContent, basicAuth).Methods("GET")
	if !readonly {
		s.handleFunc(contentRoutes, "/{name}", apilcp.StoreContent).Methods("POST")
		s.handlePrivateFunc(contentRoutes, "/{key}", apilcp.AddContent, basicAuth).Methods("PUT")
		s.handlePrivateFunc(contentRoutes, "/{content_id}/licenses", apilcp.GenerateLicense, basicAuth).Methods("POST")
		s.handlePrivateFunc(contentRoutes, "/{content_id}/publications", apilcp.GenerateProtectedPublication, basicAuth).Methods("POST")
		s.handlePrivateFunc(contentRoutes, "/{content_id}/publication", apilcp.GenerateProtectedPublication, basicAuth).Methods("POST")
	}

	licenseRoutesPathPrefix := "/licenses"
	licenseRoutes := sr.R.PathPrefix(licenseRoutesPathPrefix).Subrouter().StrictSlash(false)

	s.handlePrivateFunc(sr.R, licenseRoutesPathPrefix, apilcp.ListLicenses, basicAuth).Methods("GET")

	s.handlePrivateFunc(licenseRoutes, "/{license_id}", apilcp.GetLicense, basicAuth).Methods("GET")
	s.handlePrivateFunc(licenseRoutes, "/{license_id}", apilcp.GetLicense, basicAuth).Methods("POST")
	s.handlePrivateFunc(licenseRoutes, "/{license_id}/publication", apilcp.GenerateProtectedPublication, basicAuth).Methods("POST")
	if !readonly {
		s.handlePrivateFunc(licenseRoutes, "/{license_id}", apilcp.UpdateLicense, basicAuth).Methods("PATCH")
	}

	s.source.Feed(packager.Incoming)
	return s
}

type HandlerFunc func(w http.ResponseWriter, r *http.Request, s apilcp.Server)

func (s *Server) handleFunc(router *mux.Router, route string, fn HandlerFunc) *mux.Route {
	return router.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) {
		fn(w, r, s)
	})
}

type HandlerPrivateFunc func(w http.ResponseWriter, r *auth.AuthenticatedRequest, s apilcp.Server)

func (s *Server) handlePrivateFunc(router *mux.Router, route string, fn HandlerFunc, authenticator *auth.BasicAuth) *mux.Route {
	return router.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) {
		if api.CheckAuth(authenticator, w, r) {
			fn(w, r, s)
		}
	})
}