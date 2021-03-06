package generators

import (
	"fmt"
	"io/ioutil"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/Microsoft/fabrikate/core"
	"github.com/kyokomi/emoji"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

func AddNamespaceToManifests(manifests string, namespace string) (namespacedManifests string, err error) {
	splitManifest := strings.Split(manifests, "---")

	for _, manifest := range splitManifest {
		parsedManifest := make(map[interface{}]interface{})
		yaml.Unmarshal([]byte(manifest), &parsedManifest)

		// strip any empty entries
		if len(parsedManifest) == 0 {
			continue
		}

		if parsedManifest["metadata"] != nil {
			metadataMap := parsedManifest["metadata"].(map[interface{}]interface{})
			metadataMap["namespace"] = namespace
		}

		updatedManifest, err := yaml.Marshal(&parsedManifest)
		if err != nil {
			return "", err
		}

		namespacedManifests += fmt.Sprintf("---\n%s\n", updatedManifest)
	}

	return namespacedManifests, nil
}

func MakeHelmRepoPath(component *core.Component) string {
	return path.Join(component.PhysicalPath, "helm_repos", component.Name)
}

func GenerateHelmComponent(component *core.Component) (manifest string, err error) {
	log.Println(emoji.Sprintf(":truck: generating component '%s' with helm with repo %s", component.Name, component.Repo))

	configYaml, err := yaml.Marshal(&component.Config.Config)
	if err != nil {
		return "", err
	}

	helmRepoPath := MakeHelmRepoPath(component)
	absHelmRepoPath, err := filepath.Abs(helmRepoPath)
	chartPath := path.Join(absHelmRepoPath, component.Path)
	absCustomValuesPath := path.Join(chartPath, "overriddenValues.yaml")

	ioutil.WriteFile(absCustomValuesPath, configYaml, 0644)

	volumeMount := fmt.Sprintf("%s:/app/chart", chartPath)

	name := component.Name
	if component.Config.Config["name"] != nil {
		name = component.Config.Config["name"].(string)
	}

	manifests, err := exec.Command("docker", "run", "--rm", "-v", volumeMount, "alpine/helm:latest", "template", "/app/chart", "--values", "/app/chart/overriddenValues.yaml", "--name", name).Output()

	if err != nil {
		return "", err
	}

	stringManifests := string(manifests)

	// helm template doesn't support injecting namespaces, so if a namespace was configured, manually inject it.
	if component.Config.Config["namespace"] != nil {
		stringManifests, err = AddNamespaceToManifests(stringManifests, component.Config.Config["namespace"].(string))
	}

	return stringManifests, err
}

func InstallHelmComponent(component *core.Component) (err error) {
	helmRepoPath := MakeHelmRepoPath(component)
	if err := exec.Command("rm", "-rf", helmRepoPath).Run(); err != nil {
		return err
	}

	if err := exec.Command("mkdir", "-p", helmRepoPath).Run(); err != nil {
		return err
	}

	log.Println(emoji.Sprintf(":helicopter: install helm repo %s for %s into %s", component.Repo, component.Name, helmRepoPath))
	return exec.Command("git", "clone", component.Repo, helmRepoPath, "--depth", "1").Run()
}
