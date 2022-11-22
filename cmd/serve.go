package cmd

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/plentico/plenti/cmd/build"

	"github.com/plentico/plenti/common"

	"github.com/plentico/plenti/readers"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/briandowns/spinner"
	"github.com/gerald1248/httpscerts"
	"github.com/spf13/cobra"
	"golang.org/x/net/websocket"
)

// PortFlag allows users to override default port (3000) for local server
var PortFlag int

// BuildFlag can be set to false to skip building the site when starting local server
var BuildFlag bool

// SSLFlag can be set to true to serve localhost over HTTPS with SSL/TLS encryption
var SSLFlag bool

func setPort(siteConfig readers.SiteConfig) int {
	// default to  use value from config file
	port := siteConfig.Local.Port
	// Check if port is overridden by flag
	if PortFlag > 0 {
		// If dir flag exists, use it
		port = PortFlag
	}
	return port
}

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Lightweight webserver for local development",
	Long: heredoc.Doc(`
		Serve will run "plenti build" automatically to create
		a compiled version of your site.

		This defaults to a folder named "public" but you can 
		adjust this in your site config.

		You can also set a different port in your site config file.
	`),
	Run: func(cmd *cobra.Command, args []string) {

		s := spinner.New(spinner.CharSets[35], 100*time.Millisecond)

		s.Suffix = " Building..."
		s.Color("blue")
		s.Start()

		// Skip build command if BuildFlag is set to False
		if BuildFlag {
			// Run build command before starting server
			buildCmd.Run(cmd, args)
		}

		// Get settings from config file.
		siteConfig, _ := readers.GetSiteConfig(".")

		// Check flags and config for directory to build to
		buildDir := setBuildDir(siteConfig)

		// Check that the build directory exists
		if _, err := os.Stat(buildDir); os.IsNotExist(err) {
			fmt.Printf("The \"%v\" build directory does not exist, check your plenti.json file.\n", buildDir)
			log.Fatal(err)
		}
		// Watch filesystem for changes.
		gowatch(buildDir)

		common.QuitOnErr = false
		fmt.Printf("\nServing site from your \"%v\" directory.\n", buildDir)

		var fs http.Handler
		// Point to folder containing the built site
		if common.UseMemFS {

			fs = common.NewH(buildDir)

		} else {
			// Check that the build directory exists
			if _, err := os.Stat(buildDir); os.IsNotExist(err) {
				fmt.Printf("The \"%v\" build directory does not exist, check your plenti.json file.\n", buildDir)
				log.Fatal(err)
			}
			fs = http.FileServer(http.Dir(buildDir))
		}
		http.Handle("/", fs)
		// Handle local edits made via the Git-CMS
		http.HandleFunc("/postlocal", postLocal)
		// Watch filesystem for changes.
		gowatch(buildDir)
		if build.Doreload {
			// websockets
			http.Handle("/reload", websocket.Handler(wshandler))

		}
		// fs := http.FileServer(http.Dir("assets/"))

		// Check flags and config for local server port
		port := setPort(siteConfig)

		s.Stop()

		if SSLFlag {
			// Start an HTTPS webserver
			serveSSL(port)
		}

		fmt.Printf("Visit your site at http://localhost:%v/\n", port)
		// Start the HTTP webserver
		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))

	},
}

func init() {
	rootCmd.AddCommand(serveCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// serveCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// serveCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	serveCmd.Flags().IntVarP(&PortFlag, "port", "p", 0, "change port for local server")
	serveCmd.Flags().StringVarP(&OutputDirFlag, "output", "o", "", "change name of the public build directory")
	serveCmd.Flags().BoolVarP(&BuildFlag, "build", "B", true, "set \"false\" to disable build step")
	serveCmd.Flags().BoolVarP(&VerboseFlag, "verbose", "v", false, "show log messages")
	serveCmd.Flags().BoolVarP(&BenchmarkFlag, "benchmark", "b", false, "display build time statistics")
	serveCmd.Flags().BoolVarP(&MinifyFlag, "minify", "m", true, "minify JS output for faster performance")
	serveCmd.Flags().BoolVarP(&SSLFlag, "ssl", "s", false, "ssl/tls encryption to serve localhost over https")
	serveCmd.Flags().BoolVarP(&build.Doreload, "live-reload", "L", false, "Enable live reload")
	serveCmd.Flags().BoolVarP(&build.Local, "local", "l", true, "set false to emulate remote server")
	serveCmd.Flags().StringVarP(&ConfigFileFlag, "config", "c", "plenti.json", "use a custom sitewide configuration file")
	//serveCmd.Flags().BoolVarP(&common.UseMemFS, "in-memory", "M", false, "Use in memory filesystem")
}

type localChange struct {
	Action   string
	Encoding string
	File     string
	Contents string
}

func postLocal(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			fmt.Printf("Could not read 'body' from local edit: %v", err)
		}
		var localChanges []localChange
		err = json.Unmarshal(b, &localChanges)
		if err != nil {
			fmt.Printf("Could not unmarshal JSON data: %v", err)
		}
		var contents []byte
		for _, change := range localChanges {
			if change.Action == "create" || change.Action == "update" {
				contents = []byte(change.Contents)
				if change.Encoding == "base64" {
					contents, err = base64.StdEncoding.DecodeString(change.Contents)
					if err != nil {
						fmt.Printf("Could not decode base64 asset: %v", err)
					}
				}
				err = os.WriteFile(change.File, contents, os.ModePerm)
				if err != nil {
					fmt.Printf("Unable to write to local file: %v", err)
				}
			}
			if change.Action == "delete" {
				currentDir, _ := os.Getwd()
				err = os.Remove(filepath.Join(currentDir, change.File))
				if err != nil {
					fmt.Printf("Unable to delete local file: %v", err)
				}
			}
		}
	}
}

func serveSSL(port int) {
	cert, key, err := httpscerts.GenerateArrays(fmt.Sprintf("localhost:%d", port))
	if err != nil {
		log.Fatal("Error: Couldn't create https certs.")
	}

	keyPair, err := tls.X509KeyPair(cert, key)
	if err != nil {
		log.Fatal("Error: Couldn't create key pair")
	}

	var certificates []tls.Certificate
	certificates = append(certificates, keyPair)

	cfg := &tls.Config{
		MinVersion:               tls.VersionTLS12,
		PreferServerCipherSuites: true,
		Certificates:             certificates,
	}

	s := &http.Server{
		Addr:           fmt.Sprintf(":%d", port),
		Handler:        nil,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		ErrorLog:       log.New(ioutil.Discard, "", 0),
		MaxHeaderBytes: 1 << 20,
		TLSConfig:      cfg,
	}
	fmt.Printf("Visit your site at https://localhost:%v/\n", port)
	log.Fatal(s.ListenAndServeTLS("", ""))
}
