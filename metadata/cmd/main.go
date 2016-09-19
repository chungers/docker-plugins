package main

import (
	"encoding/json"
	"fmt"
	log "github.com/Sirupsen/logrus"
	plugin "github.com/chungers/docker-plugins/metadata"
	"github.com/spf13/cobra"
	"net"
	"os"
	"sync"
)

var (
	logLevel        = len(log.AllLevels) - 2
	listen          = "unix:///run/docker/plugins/metadata.sock"
	serviceHostPort = ":3131"

	// PluginName is the name of the plugin in the Docker Hub / registry
	PluginName = "NoPluginName"

	// PluginType is the name of the container image name / plugin name
	PluginType = "docker.metadataDriver/1.0"

	// PluginNamespace is the namespace of the plugin
	PluginNamespace = "chungers/metadata"

	// Version is the build release identifier.
	Version = "Unspecified"

	// Revision is the build source control revision.
	Revision = "Unspecified"
)

func info() interface{} {
	return map[string]interface{}{
		"name":      PluginName,
		"type":      PluginType,
		"namespace": PluginNamespace,
		"version":   Version,
		"revision":  Revision,
	}
}

type backend struct {
	service *plugin.Service
	update  chan<- map[string]interface{}
	proxy   *plugin.ReverseProxy
	lock    sync.RWMutex
}

func printInterfaceAddrs() {
	log.Infoln("Interfaces:")
	addrs, err := net.InterfaceAddrs()
	if err == nil {
		for i, addr := range addrs {
			log.Infoln("Interface", i, "network=", addr.Network(), "addr=", addr.String())
		}
	}
}

func main() {

	forwardHostPort := "169.254.169.254:80"

	// Top level main command...  all subcommands are designed to create the watch function
	// for the watcher, except 'version' subcommand.  After the subcommand completes, the
	// post run then begins execution of the actual watcher.
	cmd := &cobra.Command{
		Use:   "metadata",
		Short: "Metadata plugin for instance metadata",
		RunE: func(c *cobra.Command, args []string) error {

			if logLevel > len(log.AllLevels)-1 {
				logLevel = len(log.AllLevels) - 1
			} else if logLevel < 0 {
				logLevel = 0
			}
			log.SetLevel(log.AllLevels[logLevel])

			if c.Use == "version" {
				return nil
			}

			printInterfaceAddrs()

			service, err := plugin.NewService()
			if err != nil {
				return err
			}

			update := make(chan map[string]interface{})

			proxy := plugin.NewReverseProxy().SetForwardHostPort(forwardHostPort)

			backend := &backend{
				service: service,
				update:  update,
				proxy:   proxy,
			}

			log.Infoln("Starting the service")
			go service.Run(update)

			log.Infoln("Starting admin httpd")
			log.Infoln("Listening on:", listen)

			_, waitHTTP, err := plugin.StartServer(listen, adminHandlers(backend, update),
				func() error {
					log.Infoln("Shutting down.")

					if backend.service != nil {
						backend.service.Stop()
					}
					backend.service.Wait()
					log.Infoln("Service stopped")

					return nil
				})

			if err != nil {
				panic(err)
			}
			log.Infoln("Started admin httpd")

			serviceListen := fmt.Sprintf("tcp://%s", serviceHostPort)
			log.Infoln("Starting service httpd")
			log.Infoln("Listening on:", serviceListen)

			_, waitHTTP2, err := plugin.StartServer(serviceListen, serviceHandlers(backend))

			if err != nil {
				panic(err)
			}
			log.Infoln("Started service httpd")

			<-waitHTTP
			<-waitHTTP2

			close(update)
			return nil
		},
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "print build version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			buff, err := json.MarshalIndent(info(), "  ", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(buff))
			return nil
		},
	})

	cmd.PersistentFlags().StringVar(&listen, "listen", listen, "listen address (unix or tcp) for the control endpoint")
	cmd.PersistentFlags().IntVar(&logLevel, "log", logLevel, "Logging level. 0 is least verbose. Max is 5")
	cmd.PersistentFlags().StringVar(&forwardHostPort, "forward", forwardHostPort, "Forward host:port")
	cmd.PersistentFlags().StringVar(&serviceHostPort, "port", serviceHostPort, "TCP host:port for the service endpoint")

	err := cmd.Execute()
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}
}
