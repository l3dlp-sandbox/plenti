package build

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"plenti/readers"
	"regexp"
	"strings"
)

// DataSource builds json list from "content/" directory.
func DataSource(buildPath string, siteConfig readers.SiteConfig) (string, string) {

	fmt.Println("\nGathering data source from \"content/\" folder")

	nodesJSPath := buildPath + "/spa/ejected/nodes.js"
	os.MkdirAll(buildPath+"/spa/ejected", os.ModePerm)
	// Delete any previous nodes.js file.
	deleteNodesJSErr := os.Remove(nodesJSPath)
	if deleteNodesJSErr != nil {
		fmt.Println(deleteNodesJSErr)
	}

	// Set up counter for logging output.
	contentFileCounter := 0

	// Start the string that will be sent to nodejs for compiling.
	staticBuildStr := "["
	allNodesStr := "["

	// Start the new nodes.js file.
	err := ioutil.WriteFile(nodesJSPath, []byte(`const nodes = [`), 0755)
	if err != nil {
		fmt.Printf("Unable to write nodes.js file: %v", err)
	}

	// Go through all sub directories in "content/" folder.
	contentFilesErr := filepath.Walk("content", func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			// Get individual path arguments.
			parts := strings.Split(path, "/")
			contentType := parts[1]
			fileName := parts[len(parts)-1]

			// Don't add _blueprint.json or other special named files starting with underscores.
			if fileName[:1] != "_" {

				// Get the contents of the file.
				fileContentByte, readFileErr := ioutil.ReadFile(path)
				if readFileErr != nil {
					fmt.Printf("Could not read content file: %s\n", readFileErr)
				}
				fileContentStr := string(fileContentByte)

				// Remove file extension from path.
				path = strings.TrimSuffix(path, filepath.Ext(path))
				// Remove the "content" folder from path.
				path = strings.TrimPrefix(path, "content")

				destPath := buildPath + "/" + path + ".html"

				// Check for index.json outside of type declaration.
				if contentType == "index.json" {
					contentType = "index"
					path = "/"
				}

				// TODO: Need to check for path overrides from siteConfig reader.
				for contentType, slug := range siteConfig.Types {
					fmt.Printf("type is: %s\n", contentType)
					fmt.Printf("slug is: %s\n", slug)
				}
				nodeDetailsStr := "{\n" +
					"\"path\": \"" + path + "\",\n" +
					"\"type\": \"" + contentType + "\",\n" +
					"\"filename\": \"" + fileName + "\",\n" +
					"\"fields\": " + fileContentStr + "\n}"

				// Create path for source .svelte template.
				componentPath := "layout/content/" + contentType + ".svelte"

				//encodedNodeDetails := html.EscapeString(nodeDetailsStr)
				encodedNodeDetails := nodeDetailsStr
				// Remove newlines.
				reN := regexp.MustCompile(`\r?\n`)
				encodedNodeDetails = reN.ReplaceAllString(encodedNodeDetails, " ")
				// Remove tabs.
				reT := regexp.MustCompile(`\t`)
				encodedNodeDetails = reT.ReplaceAllString(encodedNodeDetails, " ")
				// Reduce extra whitespace to a single space.
				reS := regexp.MustCompile(`\s+`)
				encodedNodeDetails = reS.ReplaceAllString(encodedNodeDetails, " ")
				// Add to list of data_source files for creating static HTML.
				staticBuildStr = staticBuildStr + "{ \"node\": " + encodedNodeDetails + ", \"componentPath\": \"" + componentPath + "\", \"destPath\": \"" + destPath + "\"},"
				allNodesStr = allNodesStr + encodedNodeDetails + ","

				// Create new nodes.js file if it doesn't already exist, or add to it if it does.
				nodesJSFile, openNodesJSErr := os.OpenFile(nodesJSPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if openNodesJSErr != nil {
					fmt.Printf("Could not open nodes.js for writing: %s", openNodesJSErr)
				}
				// Write to the file with info from current file in "/content" folder.
				defer nodesJSFile.Close()
				if _, err := nodesJSFile.WriteString(nodeDetailsStr + ","); err != nil {
					log.Println(err)
				}

				// Increment counter for logging purposes.
				contentFileCounter++

			}
		}
		return nil
	})
	if contentFilesErr != nil {
		fmt.Printf("Could not get layout file: %s", contentFilesErr)
	}

	// Complete the nodes.js file.
	nodesJSFile, openNodesJSErr := os.OpenFile(nodesJSPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if openNodesJSErr != nil {
		fmt.Printf("Could not open nodes.js for writing: %s", openNodesJSErr)
	}
	defer nodesJSFile.Close()
	nodesJSStr := "];\n\nexport default nodes;"
	if _, err := nodesJSFile.WriteString(nodesJSStr); err != nil {
		log.Println(err)
	}

	// End the string that will be sent to nodejs for compiling.
	staticBuildStr = strings.TrimSuffix(staticBuildStr, ",") + "]"
	allNodesStr = strings.TrimSuffix(allNodesStr, ",") + "]"

	fmt.Printf("Number of content files used: %d\n", contentFileCounter)

	return staticBuildStr, allNodesStr

}
