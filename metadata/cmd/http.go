package main

import (
	"encoding/json"
	log "github.com/Sirupsen/logrus"
	plugin "github.com/chungers/docker-plugins/metadata"
	"github.com/gorilla/mux"
	"io/ioutil"
	"net/http"
)

func metadataGetHandler(backend *backend) func(resp http.ResponseWriter, req *http.Request) {
	return func(resp http.ResponseWriter, req *http.Request) {
		vars := mux.Vars(req)

		log.Infoln(PluginName, " - Get requested via http. Path=", vars["path"])

		if backend.service != nil {
			v := backend.service.Get(vars["path"])
			if v != nil {

				switch v := v.(type) {
				case string:
					resp.Write([]byte(v))
					return
				default:
					buff, err := json.MarshalIndent(v, "  ", "  ")
					if noError(err, resp) {
						resp.Write(buff)
					}
					return
				}
			}
		}

		backend.lock.RLock()
		defer backend.lock.RUnlock()

		// forward to backend
		log.Infoln(PluginName, " - forwarding", vars["path"], "to", backend.proxy.ProxyURL())

		backend.proxy.ServeHTTP(resp, req)
		return
	}
}

// serviceHandlers returns readonly service handlers
func serviceHandlers(backend *backend) http.Handler {
	router := mux.NewRouter()
	router.HandleFunc("/{path:.*}", metadataGetHandler(backend)).Methods("GET")
	return router
}

// adminHandlers returns a http handler for the admin service over the tcp/unix listen address
func adminHandlers(backend *backend, update chan<- map[string]interface{}) http.Handler {
	router := mux.NewRouter()

	// Get the info
	router.HandleFunc("/v1/info",
		func(resp http.ResponseWriter, req *http.Request) {
			log.Infoln("Request for info")
			buff, err := json.MarshalIndent(info(), "  ", "  ")
			if noError(err, resp) {
				_, err = resp.Write(buff)
				if noError(err, resp) {
					return
				}
			}
			return
		}).Methods("GET")

	// Updates the configuration such as the host:port of the proxy to forward to
	router.HandleFunc("/MetadataDriver.Config",
		func(resp http.ResponseWriter, req *http.Request) {
			log.Infoln(PluginName, " - Update config via http.")

			if backend.proxy == nil {
				log.Warningln("cannot update")
				resp.WriteHeader(http.StatusMethodNotAllowed)
				return
			}

			buff, err := ioutil.ReadAll(req.Body)
			if noError(err, resp) {

				config := struct {
					ForwardHostPort string `json:"forward_hostport"`
				}{}

				err := json.Unmarshal(buff, &config)
				if noError(err, resp) {

					backend.lock.Lock()
					defer backend.lock.Unlock()

					backend.proxy = plugin.NewReverseProxy().SetForwardHostPort(config.ForwardHostPort)
					log.Infoln("proxy updated to", config.ForwardHostPort)
				}
			}
			return
		}).Methods("POST")

	router.HandleFunc("/{path:.*}",
		func(resp http.ResponseWriter, req *http.Request) {
			vars := mux.Vars(req)

			log.Infoln(PluginName, " - Update requested via http. Path=", vars["path"])
			if backend.update == nil {
				log.Warningln("cannot update")
				resp.WriteHeader(http.StatusMethodNotAllowed)
				return
			}

			buff, err := ioutil.ReadAll(req.Body)
			if noError(err, resp) {

				// if we can parse as map then send as map of key values
				// otherwise use the path as key and content as value
				m := map[string]interface{}{}

				err := json.Unmarshal(buff, &m)
				if err == nil {
					backend.update <- m
				} else {
					backend.update <- map[string]interface{}{
						vars["path"]: string(buff),
					}
				}
				log.Infoln("Dispatched configuration.")
				return
			}
			return
		}).Methods("PUT")

	// get value
	router.HandleFunc("/{path:.*}", metadataGetHandler(backend)).Methods("GET")

	return router
}

func noError(err error, resp http.ResponseWriter) bool {
	if err != nil {
		log.Warningln("error=", err)
		resp.WriteHeader(http.StatusInternalServerError)
		return false
	}
	return true
}
