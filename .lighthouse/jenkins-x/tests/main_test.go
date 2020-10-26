package tests

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jenkins-x/go-scm/scm"
	v1 "github.com/jenkins-x/jx-api/v3/pkg/apis/jenkins.io/v1"
	"github.com/jenkins-x/jx-api/v3/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx-application/pkg/applications"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cmdrunner"
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient"
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient/cli"
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient/giturl"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jobs"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxclient"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/naming"
	"github.com/jenkins-x/jx-helpers/v3/pkg/scmhelpers"
	"github.com/jenkins-x/jx-helpers/v3/pkg/stringhelpers"
	"github.com/jenkins-x/jx-helpers/v3/pkg/termcolor"
	"github.com/jenkins-x/jx-promote/pkg/environments"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var (
	info = termcolor.ColorInfo

	removePaths = []string{".lighthouse", "jenkins-x.yml", "charts", "preview", "Dockerfile"}
)

func TestPipelineCatalogWorksOnTestRepository(t *testing.T) {
	repoName := os.Getenv("JOB_NAME")
	require.NotEmpty(t, repoName, "no $JOB_NAME defined")

	packDir, err := filepath.Abs("../../../packs")
	require.NoError(t, err, "failed to find pack dir")
	t.Logf("using packs dir %s", packDir)

	o := &Options{
		T:                      t,
		Repository:             repoName,
		PackDir:                packDir,
		PullRequestPollTimeout: 20 * time.Minute,
		PullRequestPollPeriod:  10 * time.Second,
		InsecureURLSkipVerify:  true,
	}
	o.Run()
}

type Options struct {
	T                      *testing.T
	Owner                  string
	Repository             string
	GitURL                 string
	PackDir                string
	Namespace              string
	MainBranch             string
	ReleaseBuildNumber     string
	MergeSHA               string
	GitOperatorNamespace   string
	InsecureURLSkipVerify  bool
	Verbose                bool
	GitClient              gitclient.Interface
	CommandRunner          cmdrunner.CommandRunner
	ScmFactory             scmhelpers.Factory
	PullRequestPollTimeout time.Duration
	PullRequestPollPeriod  time.Duration
	KubeClient             kubernetes.Interface
	JXClient               versioned.Interface
}

// Validate verifies we can lazily create the various clients
func (o *Options) Validate() {
	if o.GitOperatorNamespace == "" {
		o.GitOperatorNamespace = "jx-git-operator"
	}
	if o.MainBranch == "" {
		o.MainBranch = "master"
	}
	if o.Owner == "" {
		o.Owner = "jenkins-x-labs-bdd-tests"
	}
	if o.ScmFactory.GitServerURL == "" {
		o.ScmFactory.GitServerURL = giturl.GitHubURL
	}
	if o.GitURL == "" {
		o.GitURL = stringhelpers.UrlJoin(o.ScmFactory.GitServerURL, o.Owner, o.Repository)
	}

	var err error
	if o.CommandRunner == nil {
		o.CommandRunner = cmdrunner.QuietCommandRunner
	}
	if o.GitClient == nil {
		o.GitClient = cli.NewCLIClient("", o.CommandRunner)
	}

	if o.ScmFactory.ScmClient == nil {
		_, err = o.ScmFactory.Create()
		require.NoError(o.T, err, "failed to create ScmClient")
	}

	o.JXClient, o.Namespace, err = jxclient.LazyCreateJXClientAndNamespace(o.JXClient, o.Namespace)
	require.NoError(o.T, err, "failed to create the jx client")
	o.KubeClient, err = kube.LazyCreateKubeClient(o.KubeClient)
	require.NoError(o.T, err, "failed to create the kube client")
}

// Run runs the test suite
func (o *Options) Run() {
	o.Validate()

	pr := o.CreatePullRequest()

	buildNumber := o.findNextBuildNumber()

	o.waitForPullRequestToMerge(pr)

	o.verifyPreviewEnvironment(pr)

	releasePA := o.waitForReleasePipelineToComplete(buildNumber)
	o.waitForPromotePullRequestToMerge(releasePA)
}

// CreatePullRequest creates the pull request with the new build pack
func (o *Options) CreatePullRequest() *scm.PullRequest {
	t := o.T

	pro := &environments.EnvironmentPullRequestOptions{
		ScmClientFactory:  o.ScmFactory,
		Gitter:            o.GitClient,
		CommandRunner:     o.CommandRunner,
		GitKind:           o.ScmFactory.GitKind,
		OutDir:            "",
		BranchName:        "",
		PullRequestNumber: 0,
		CommitTitle:       "fix: test out pipeline catalog changes",
		CommitMessage:     "",
		ScmClient:         o.ScmFactory.ScmClient,
		BatchMode:         true,
		UseGitHubOAuth:    false,
		Fork:              false,
	}

	pro.Function = func() error {
		dir := pro.OutDir

		o.Infof("cloned to git dir %s", dir)

		for _, p := range removePaths {
			path := filepath.Join(dir, p)
			err := os.RemoveAll(path)
			if err != nil {
				return errors.Wrapf(err, "failed to remove %s", path)
			}
			o.Debugf("removed %s\n", path)
		}

		c := &cmdrunner.Command{
			Dir:  dir,
			Name: "jx",
			Args: []string{"project", "import", "--dry-run", "--batch-mode", "--pipeline-catalog-dir", o.PackDir},
		}
		_, err := o.CommandRunner(c)
		if err != nil {
			return errors.Wrapf(err, "failed to run %s", c.CLI())
		}
		t.Logf("regenerated the pipeline catalog in dir %s", dir)
		return nil
	}

	prDetails := &scm.PullRequest{}

	pr, err := pro.Create(o.GitURL, "", prDetails, true)
	require.NoError(t, err, "failed to create Pull Request on git repository %s", o.GitURL)
	require.NotNil(t, pr, "no PullRequest returned for repository %s", o.GitURL)

	prURL := pr.Link

	t.Logf("created Pull Request: %s", info(prURL))
	return pr
}

// PollLoop polls the given callback until the poll period expires or the function returns true
func (o *Options) PollLoop(pollTimeout, pollPeriod time.Duration, message string, fn func(elapsed time.Duration) (bool, error)) error {
	start := time.Now()
	end := start.Add(pollTimeout)
	durationString := pollTimeout.String()

	o.Infof("Waiting up to %s for %s...", durationString, message)

	for {
		elapsed := time.Now().Sub(start)
		flag, err := fn(elapsed)
		if err != nil {
			return errors.Wrapf(err, "failed to invoke function")
		}
		if flag {
			return nil
		}

		if time.Now().After(end) {
			return fmt.Errorf("Timed out waiting for %s. Waited %s", message, durationString)
		}
		time.Sleep(pollPeriod)
	}
}

func (o *Options) Debugf(message string, args ...interface{}) {
	if o.Verbose {
		o.Infof("DEBUG: "+message, args...)
	}
}

func (o *Options) Infof(message string, args ...interface{}) {
	o.T.Logf(message+"\n", args...)
}

func (o *Options) Warnf(message string, args ...interface{}) {
	o.Infof("WARN: "+message, args...)
}

// ActivitySelector returns the activity selector for the repo and branch
func (o *Options) ActivitySelector(branch string) string {
	return "owner=" + naming.ToValidName(o.Owner) + ",repository=" + naming.ToValidName(o.Repository) + ",branch=" + naming.ToValidValue(branch)
}

func (o *Options) findNextBuildNumber() string {
	t := o.T
	_, buildNumber, _, err := o.getLatestPipelineActivity(o.MainBranch)
	require.NoError(t, err, "failed to find latest PipelineActivity for branch %s", o.MainBranch)

	buildNumber++
	o.ReleaseBuildNumber = strconv.Itoa(buildNumber)
	o.Infof("next PipelineActivity release build number is: #%s", o.ReleaseBuildNumber)
	return o.ReleaseBuildNumber
}

func (o *Options) waitForReleasePipelineToComplete(buildNumber string) *v1.PipelineActivity {
	t := o.T
	jxClient := o.JXClient
	ns := o.Namespace
	ctx := context.Background()
	selector := o.ActivitySelector(o.MainBranch)

	lastStatusString := ""
	var answer *v1.PipelineActivity
	fn := func(elapsed time.Duration) (bool, error) {
		resources, err := jxClient.JenkinsV1().PipelineActivities(ns).List(ctx, metav1.ListOptions{LabelSelector: selector})
		if err != nil && apierrors.IsNotFound(err) {
			err = nil
		}
		if err != nil {
			return false, errors.Wrapf(err, "failed to list PipelineActivity resources in namespace %s with selector %s", ns, selector)
		}

		for i := range resources.Items {
			r := &resources.Items[i]
			buildName := r.Spec.Build
			if buildName != buildNumber {
				continue
			}
			ps := &r.Spec

			status := string(ps.Status)
			if status != lastStatusString {
				lastStatusString = status
				o.Infof("PipelineActivity %s has status %s", info(r.Name), info(status))
			}

			if ps.Status.IsTerminated() {
				answer = r
				return true, nil
			}
		}
		return false, nil
	}

	message := fmt.Sprintf("release complete for PipelineActivity build %s with selector %s", info(o.ReleaseBuildNumber), info(selector))
	err := o.PollLoop(o.PullRequestPollTimeout, o.PullRequestPollPeriod, message, fn)
	require.NoError(t, err, "failed to %s", message)

	require.NotNil(t, answer, "no PipelineActivity found for %s", message)
	require.Equal(t, v1.ActivityStatusTypeSucceeded, answer.Spec.Status, "status for %s", message)
	return answer
}

func (o *Options) getLatestPipelineActivity(branch string) (pa *v1.PipelineActivity, buildNumber int, selector string, err error) {
	jxClient := o.JXClient
	ns := o.Namespace
	ctx := context.Background()

	selector = o.ActivitySelector(branch)
	var resources *v1.PipelineActivityList
	resources, err = jxClient.JenkinsV1().PipelineActivities(ns).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil && apierrors.IsNotFound(err) {
		err = nil
	}
	if err != nil {
		return
	}

	for i := range resources.Items {
		r := &resources.Items[i]
		buildName := r.Spec.Build
		if buildName != "" {
			b, err := strconv.Atoi(buildName)
			if err != nil {
				o.Warnf("failed to convert build number %s to number for PipelineActivity %s: %s", buildName, r.Name, err.Error())
				continue
			}
			if b > buildNumber {
				buildNumber = b
				pa = r
			}
		}
	}
	return
}

func (o *Options) verifyPreviewEnvironment(pr *scm.PullRequest) {
	t := o.T
	branch := fmt.Sprintf("PR-%d", pr.Number)
	pa, _, selector, err := o.getLatestPipelineActivity(branch)
	require.NoError(t, err, "failed to find latest PipelineActivity for branch %s", branch)
	require.NotNil(t, pa, "could not find a PipelineActivity for selector %s", selector)

	previewURL := ""
	for i := range pa.Spec.Steps {
		s := &pa.Spec.Steps[i]
		preview := s.Preview
		if preview != nil {
			previewURL = preview.ApplicationURL
			if previewURL != "" {
				break
			}
		}
	}
	require.NotEmpty(t, previewURL, "could not find a Preview URL for PipelineActivity %s", pa.Name)

	o.Infof("found preview URL: %s", info(previewURL))

	statusCode := o.GetAppHttpStatusCode()
	o.AssertURLReturns(previewURL, statusCode, o.PullRequestPollTimeout, o.PullRequestPollPeriod)
}

func (o *Options) GetAppHttpStatusCode() int {
	statusCode := 200
	// spring quickstarts return 404 for the home page
	if strings.HasPrefix(o.Repository, "spring") {
		statusCode = 404
	}
	return statusCode
}

// ExpectUrlReturns expects that the given URL returns the given status code within the given time period
func (o *Options) AssertURLReturns(url string, expectedStatusCode int, pollTimeout, pollPeriod time.Duration) error {
	lastLogMessage := ""
	logMessage := func(message string) {
		if message != lastLogMessage {
			lastLogMessage = message
			o.Infof(message)
		}
	}

	fn := func(elapsed time.Duration) (bool, error) {
		actualStatusCode, err := o.GetURLStatusCode(url, logMessage)
		if err != nil {
			return false, nil
		}
		return actualStatusCode == expectedStatusCode, nil
	}
	message := fmt.Sprintf("expecting status %d on URL %s", expectedStatusCode, url)
	return o.PollLoop(pollTimeout, pollPeriod, message, fn)
}

// GetURLStatusCode gets the URL status code
func (o *Options) GetURLStatusCode(url string, logMessage func(message string)) (int, error) {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: o.InsecureURLSkipVerify,
		},
	}
	var httpClient = &http.Client{
		Timeout:   time.Second * 30,
		Transport: transport,
	}
	response, err := httpClient.Get(url)
	if err != nil {
		errorMessage := err.Error()
		if response != nil {
			errorMessage += " status: " + response.Status
		}
		message := fmt.Sprintf("failed to invoke URL %s got: %s", info(url), errorMessage)
		logMessage(message)
		return 0, errors.Wrap(err, message)
	}
	actualStatusCode := response.StatusCode
	logMessage(fmt.Sprintf("invoked URL %s and got return code: %s", info(url), info(strconv.Itoa(actualStatusCode))))
	return actualStatusCode, nil
}

func (o *Options) waitForPullRequestToMerge(pullRequest *scm.PullRequest) *scm.PullRequest {
	t := o.T
	logNoMergeCommitSha := false
	logHasMergeSha := false
	message := fmt.Sprintf("pull request %s to merge", info(pullRequest.Link))

	ctx := context.Background()
	fullName := pullRequest.Repository().FullName
	prNumber := pullRequest.Number

	o.MergeSHA = ""
	var err error
	var pr *scm.PullRequest
	fn := func(elapsed time.Duration) (bool, error) {
		pr, _, err = o.ScmFactory.ScmClient.PullRequests.Find(ctx, fullName, prNumber)
		if err != nil {
			o.Warnf("Failed to query the Pull Request status for %s %s", pullRequest.Link, err)
		} else {
			if pr.MergeSha != "" {
				o.MergeSHA = pr.MergeSha
			}
			elaspedString := elapsed.String()
			if pr.Merged {
				if pr.MergeSha == "" && o.MergeSHA == "" {
					if !logNoMergeCommitSha {
						logNoMergeCommitSha = true
						o.Infof("Pull Request %s is merged but we don't yet have a merge SHA after waiting %s", info(pr.Link), elaspedString)
						return false, nil
					}
				} else {
					if !logHasMergeSha {
						logHasMergeSha = true
						o.Infof("Pull Request %s is merged at sha %s after waiting %s", info(pr.Link), info(o.MergeSHA), elaspedString)
						return true, nil
					}
				}
			} else {
				if pr.Closed {
					o.Warnf("Pull Request %s is closed after waiting %s", info(pr.Link), elaspedString)
					return true, nil
				}
			}
		}
		return false, nil
	}

	err = o.PollLoop(o.PullRequestPollTimeout, o.PullRequestPollPeriod, message, fn)
	require.NoError(t, err, "failed to %s", message)

	return pr
}

func (o *Options) waitForPromotePullRequestToMerge(pa *v1.PipelineActivity) {
	t := o.T

	version := pa.Spec.Version
	prURL := ""
	for i := range pa.Spec.Steps {
		s := &pa.Spec.Steps[i]
		promote := s.Promote
		if promote != nil && promote.PullRequest != nil {
			prURL = promote.PullRequest.PullRequestURL
			if prURL != "" {
				break
			}
		}
	}
	require.NotEmpty(t, version, "could not find the version for PipelineActivity %s", pa.Name)
	require.NotEmpty(t, prURL, "could not find the Promote PullRequest URL for PipelineActivity %s", pa.Name)

	o.Infof("found Promote Pull Request: %s", info(prURL))

	pr, err := scmhelpers.ParsePullRequestURL(prURL)
	require.NoError(t, err, "failed to parse Pull Request: %s", prURL)

	o.waitForPullRequestToMerge(pr)

	require.NotEmpty(t, o.MergeSHA, "no merge SHA for the promote Pull Request %s", prURL)

	o.waitForSuccessfulBootJob(o.MergeSHA)

	o.waitForVersionInStaging(version)
}

func (o *Options) waitForSuccessfulBootJob(sha string) {
	t := o.T
	selector := "app=jx-boot,git-operator.jenkins.io/commit-sha=" + sha

	message := fmt.Sprintf("successful Job in namespace %s with selector %s", info(o.GitOperatorNamespace), info(selector))
	ctx := context.Background()
	ns := o.GitOperatorNamespace
	kubeClient := o.KubeClient

	lastStatus := ""
	fn := func(elapsed time.Duration) (bool, error) {
		resources, err := kubeClient.BatchV1().Jobs(ns).List(ctx, metav1.ListOptions{LabelSelector: selector})
		if err != nil && apierrors.IsNotFound(err) {
			err = nil
		}
		if err != nil {
			return false, errors.Wrapf(err, "failed to list Jobs in namespace %s with selector %s", ns, selector)
		}

		jobName := ""
		answer := false
		status := "Pending"
		count := len(resources.Items)
		if count == 0 {
			status = fmt.Sprintf("no jobs found matching selector %s", selector)
		} else {
			if count > 1 {
				o.Warnf("found %s Jobs in namespace %s with selector %s", count, ns, selector)
			}

			// lets use the last one
			job := &resources.Items[count-1]
			jobName = job.Name
			if jobs.IsJobFinished(job) {
				if jobs.IsJobSucceeded(job) {
					status = "Succeeded"
					answer = true
				} else {
					status = "Failed"
					err = errors.Errorf("job %s has failed", job.Name)
				}
			} else {
				if job.Status.Active > 0 {
					status = "Running"
				}
			}
		}
		if status != lastStatus {
			lastStatus = status
			if jobName != "" {
				o.Infof("boot Job %s has status: %s", info(jobName), info(status))
			} else {
				o.Infof("status: %s", info(status))
			}
		}
		return answer, err
	}

	err := o.PollLoop(o.PullRequestPollTimeout, o.PullRequestPollPeriod, message, fn)
	require.NoError(t, err, "failed to poll for completed Job in namespace %s for selector %s", ns, selector)
}

func (o *Options) waitForVersionInStaging(version string) {
	t := o.T
	message := fmt.Sprintf("waiting for version %s to be in Staging", info(version))
	ns := o.Namespace

	expectedStatusCode := o.GetAppHttpStatusCode()
	lastStatus := ""
	fn := func(elapsed time.Duration) (bool, error) {
		list, err := applications.GetApplications(o.JXClient, o.KubeClient, ns)
		if err != nil {
			return false, errors.Wrap(err, "fetching applications")
		}
		answer := false
		status := ""
		if len(list.Items) == 0 {
			status = "No applications found"
		}
		for i := range list.Items {
			app := &list.Items[i]
			name := app.Name()
			if !strings.HasPrefix(name, o.Repository) {
				continue
			}
			envs := app.Environments
			if envs != nil {
				env := envs["staging"]
				depName := ""
				foundVersion := ""
				for j := range env.Deployments {
					dep := &env.Deployments[j]
					depVersion := dep.Version()

					if version == depVersion {
						appURL := dep.URL(o.KubeClient, app)
						status = fmt.Sprintf("has version %s running in staging at: %s", version, appURL)

						if appURL != "" {
							logMessage := func(message string) {
								status += " " + message
							}
							actualStatusCode, err := o.GetURLStatusCode(appURL, logMessage)
							if err != nil {
								o.Warnf("failed to get URL %s")
							} else {
								status += fmt.Sprintf(" got status code %d", actualStatusCode)
								if actualStatusCode == expectedStatusCode {
									answer = true
								}
							}
						}
						break
					} else {
						foundVersion = depVersion
						depName = dep.Name
					}
				}
				if !answer {
					o.Infof("app %s has deployment %s with version %s", name, depName, foundVersion)
				}
			}
		}
		if status != lastStatus {
			lastStatus = status
			o.Infof(status)
		}
		return answer, nil
	}
	err := o.PollLoop(o.PullRequestPollTimeout, o.PullRequestPollPeriod, message, fn)
	require.NoError(t, err, "failed to wait for version %s to be in Staging", ns, version)
}
