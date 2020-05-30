package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"io/ioutil"

	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-tools/go-steputils/stepconf"
)

type config struct {
	AuthToken     string `env:"auth_token,required"`
	RepositoryURL string `env:"repository_url,required"`
	CommitHash    string `env:"commit_hash,required"`
	APIURL        string `env:"api_base_url"`

	State            string `env:"set_specific_status,opt[auto,pending,success,error,failure]"`
	BuildURL         string `env:"build_url"`
	StatusIdentifier string `env:"status_identifier"`
	Description      string `env:"description"`
	Verbose          bool   `env:"verbose"`
}

type deploymentRequest struct {
	RequiredContexts []string   `json:"required_contexts"`
	Ref         string `json:"ref"`
	Environment string `json:"environment"`
	State       string `json:"state"`
	TargetURL   string `json:"target_url,omitempty"`
	Description string `json:"description,omitempty"`
	Context     string `json:"context,omitempty"`
}

type deploymentStatusRequest struct {
	EnvironmentUrl string `json:"environment_url"`
	Environment string `json:"environment"`
	State       string `json:"state"`
	Description string `json:"description,omitempty"`
}

// ownerAndRepo returns the owner and the repository part of a git repository url. Possible url formats:
// - https://hostname/owner/repository.git
// - git@hostname:owner/repository.git
func ownerAndRepo(url string) (string, string) {
	url = strings.TrimPrefix(strings.TrimPrefix(url, "https://"), "git@")
	a := strings.FieldsFunc(url, func(r rune) bool { return r == '/' || r == ':' })
	return a[1], strings.TrimSuffix(a[2], ".git")
}

func getState(preset string) string {
	if preset != "auto" {
		return preset
	}
	if os.Getenv("BITRISE_BUILD_STATUS") == "0" {
		return "success"
	}
	return "failure"
}

func getDescription(desc, state string) string {
	if desc == "" {
		strings.Title(getState(state))
	}
	return desc
}

func httpDump(req *http.Request, resp *http.Response) (string, error) {
	responseStr, err := httputil.DumpResponse(resp, true)
	if err != nil {
		return "", fmt.Errorf("unable to dump response, error: %s", err)
	}

	requestStr, err := httputil.DumpRequest(req, true)
	if err != nil {
		return "", fmt.Errorf("unable to dump request, error: %s", err)
	}

	return "Request: " + string(requestStr) + "\nResponse: " + string(responseStr), nil
}



// createDeployment creates a commit status for the given commit.
// see also: https://developer.github.com/v3/repos/deployments/#create-a-deployment
// POST /repos/:owner/:repo/statuses/:sha
func createDeployment(cfg config) error {
	owner, repo := ownerAndRepo(cfg.RepositoryURL)
	url := fmt.Sprintf("%s/repos/%s/%s/deployments", cfg.APIURL, owner, repo)
	body, err := json.Marshal(deploymentRequest{
		Ref:         cfg.CommitHash,
		Environment: "staging",
		Description: getDescription(cfg.Description, cfg.State),
		RequiredContexts: make([]string, 0),
	})

	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Add("Authorization", "token "+cfg.AuthToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send the request: %s", err)
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Errorf("Error when closing HTTP response body:", err)
		}
	}()

	if resp.StatusCode != 201 || cfg.Verbose {
		d, err := httpDump(req, resp)
		if err != nil {
			return err
		}
		fmt.Println(d)
	}

	if resp.StatusCode != 201 {
		return fmt.Errorf("server error, unexpected status code: %s", resp.Status)
	}


	type Response struct {
		Id  int `json: "id"`
		Url string `json: "url"`
	}
	

	body, err2 := ioutil.ReadAll(resp.Body)
	
	if err2 != nil {
		panic(err.Error())
	}

	var response Response
	json.Unmarshal(body, &response)

	var deploymentId int = response.Id

	fmt.Println("deployment id", deploymentId)

	return createDeploymentStatus(cfg, deploymentId)

}


func createDeploymentStatus(cfg config, deploymentId int) error {
	owner, repo := ownerAndRepo(cfg.RepositoryURL)
	url := fmt.Sprintf("%s/repos/%s/%s/deployments/%d/statuses", cfg.APIURL, owner, repo, deploymentId)

	body, err := json.Marshal(deploymentStatusRequest{
		Environment:     "staging",
		Description:     getDescription(cfg.Description, cfg.State),
		State:           getState(cfg.State),
		EnvironmentUrl:  cfg.BuildURL,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Add("Authorization", "token "+cfg.AuthToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send the request: %s", err)
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Errorf("Error when closing HTTP response body:", err)
		}
	}()

	if resp.StatusCode != 201 || cfg.Verbose {
		d, err := httpDump(req, resp)
		if err != nil {
			return err
		}
		fmt.Println(d)
	}

	if resp.StatusCode != 201 {
		return fmt.Errorf("server error, unexpected status code: %s", resp.Status)
	}

	return nil
}




func main() {
	var cfg config
	if err := stepconf.Parse(&cfg); err != nil {
		log.Errorf("Error: %s\n", err)
		os.Exit(1)
	}
	stepconf.Print(cfg)

	if err := createDeployment(cfg); err != nil {
		log.Errorf("Error: %s\n", err)
		os.Exit(1)
	}

}
