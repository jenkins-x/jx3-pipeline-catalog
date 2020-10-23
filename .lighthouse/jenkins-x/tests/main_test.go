package tests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/jenkins-x/go-scm/scm"
	v1 "github.com/jenkins-x/jx-api/v3/pkg/apis/jenkins.io/v1"
	"github.com/jenkins-x/jx-api/v3/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx-helpers/v3/pkg/cmdrunner"
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient"
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient/cli"
	"github.com/jenkins-x/jx-helpers/v3/pkg/gitclient/giturl"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/jxclient"
	"github.com/jenkins-x/jx-helpers/v3/pkg/kube/naming"
	"github.com/jenkins-x/jx-helpers/v3/pkg/scmhelpers"
	"github.com/jenkins-x/jx-helpers/v3/pkg/stringhelpers"
	"github.com/jenkins-x/jx-helpers/v3/pkg/termcolor"
	"github.com/jenkins-x/jx-logging/v3/pkg/log"
	"github.com/jenkins-x/jx-promote/pkg/environments"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	info = termcolor.ColorInfo

	removePaths = []string{".lighthouse", "jenkins-x.yml", "charts", "preview", "Dockerfile"}
)

func TestInitialPipelineActivity(t *testing.T) {
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
	Selector               string
	ReleaseBuildNumber     string
	GitClient              gitclient.Interface
	CommandRunner          cmdrunner.CommandRunner
	ScmFactory             scmhelpers.Factory
	PullRequestPollTimeout time.Duration
	PullRequestPollPeriod  time.Duration
	JXClient               versioned.Interface
}

// Validate verifies we can lazily create the various clients
func (o *Options) Validate() {
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
		o.CommandRunner = cmdrunner.DefaultCommandRunner
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
}

// Run runs the test suite
func (o *Options) Run() {
	o.Validate()

	pr := o.CreatePullRequest()

	buildNumber := o.findNextBuildNumber()

	o.waitForPullRequestToMerge(pr)

	o.waitForReleasePipelineToComplete(buildNumber)
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

		t.Logf("cloned to git dir %s", dir)

		for _, p := range removePaths {
			path := filepath.Join(dir, p)
			err := os.RemoveAll(path)
			if err != nil {
				return errors.Wrapf(err, "failed to remove %s", path)
			}
			t.Logf("removed %s\n", path)
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

	t.Logf("created Pull Request %s", prURL)
	return pr
}

func (o *Options) waitForPullRequestToMerge(pullRequestInfo *scm.PullRequest) *scm.PullRequest {
	logNoMergeCommitSha := false
	logHasMergeSha := false

	t := o.T
	message := fmt.Sprintf("pull request %s to merge", info(pullRequestInfo.Link))

	ctx := context.Background()
	fullName := pullRequestInfo.Repository().FullName
	prNumber := pullRequestInfo.Number

	var err error
	var pr *scm.PullRequest
	fn := func(elapsed time.Duration) (bool, error) {
		pr, _, err = o.ScmFactory.ScmClient.PullRequests.Find(ctx, fullName, prNumber)
		if err != nil {
			o.Warnf("Failed to query the Pull Request status for %s %s", pullRequestInfo.Link, err)
		} else {
			elaspedString := elapsed.String()
			if pr.Merged {
				if pr.MergeSha == "" {
					if !logNoMergeCommitSha {
						logNoMergeCommitSha = true
						o.Infof("Pull Request %s is merged but we don't yet have a merge SHA after waiting %s", termcolor.ColorInfo(pr.Link), elaspedString)
						return true, nil
					}
				} else {
					mergeSha := pr.MergeSha
					if !logHasMergeSha {
						logHasMergeSha = true
						o.Infof("Pull Request %s is merged at sha %s after waiting %s", termcolor.ColorInfo(pr.Link), termcolor.ColorInfo(mergeSha), elaspedString)
						return true, nil
					}
				}
			} else {
				if pr.Closed {
					o.Warnf("Pull Request %s is closed after waiting %s", termcolor.ColorInfo(pr.Link), elaspedString)
					return true, nil
				}
			}
		}
		return false, nil
	}

	err = PollLoop(o.PullRequestPollTimeout, o.PullRequestPollPeriod, message, fn)
	require.NoError(t, err, "failed to %s", message)

	return pr
}

// PollLoop polls the given callback until the poll period expires or the function returns true
func PollLoop(pollTimeout, pollPeriod time.Duration, message string, fn func(elapsed time.Duration) (bool, error)) error {
	start := time.Now()
	end := start.Add(pollTimeout)
	durationString := pollTimeout.String()

	log.Logger().Infof("Waiting up to %s for %s...", durationString, message)

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

func (o *Options) Infof(message string, args ...interface{}) {
	o.T.Logf(message+"\n", args...)
}

func (o *Options) Warnf(message string, args ...interface{}) {
	o.Infof("WARN: "+message, args...)
}

func (o *Options) findNextBuildNumber() string {
	t := o.T
	jxClient := o.JXClient
	ns := o.Namespace
	ctx := context.Background()
	if o.Selector == "" {
		o.Selector = "owner=" + naming.ToValidName(o.Owner) + ",repository=" + naming.ToValidName(o.Repository) + ",branch=master"
	}
	resources, err := jxClient.JenkinsV1().PipelineActivities(ns).List(ctx, metav1.ListOptions{LabelSelector: o.Selector})
	if err != nil && apierrors.IsNotFound(err) {
		err = nil
	}
	require.NoError(t, err, "failed to list PipelineActivity resources in namespace %s with selector %s", ns, o.Selector)

	maxBuildNumber := 0
	for _, r := range resources.Items {
		buildName := r.Spec.Build
		if buildName != "" {
			b, err := strconv.Atoi(buildName)
			if err != nil {
				o.Warnf("failed to convert build number %s to number for PipelineActivity %s: %s", buildName, r.Name, err.Error())
				continue
			}
			if b > maxBuildNumber {
				maxBuildNumber = b
			}
		}
	}
	maxBuildNumber++
	o.ReleaseBuildNumber = strconv.Itoa(maxBuildNumber)
	o.Infof("next PipelineActivity release build number is: #s", o.ReleaseBuildNumber)
	return o.ReleaseBuildNumber
}

func (o *Options) waitForReleasePipelineToComplete(buildNumber string) *v1.PipelineActivity {
	t := o.T
	jxClient := o.JXClient
	ns := o.Namespace
	ctx := context.Background()

	var answer *v1.PipelineActivity
	fn := func(elapsed time.Duration) (bool, error) {
		resources, err := jxClient.JenkinsV1().PipelineActivities(ns).List(ctx, metav1.ListOptions{LabelSelector: o.Selector})
		if err != nil && apierrors.IsNotFound(err) {
			err = nil
		}
		if err != nil {
			return false, errors.Wrapf(err, "failed to list PipelineActivity resources in namespace %s with selector %s", ns, o.Selector)
		}

		lastStatusString := ""
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

	message := fmt.Sprintf("release complete for PipelineActivity build %s with selector %s", info(o.ReleaseBuildNumber), info(o.Selector))
	err := PollLoop(o.PullRequestPollTimeout, o.PullRequestPollPeriod, message, fn)
	require.NoError(t, err, "failed to %s", message)

	require.NotNil(t, answer, "no PipelineActivity found for %s", message)
	require.Equal(t, v1.ActivityStatusTypeSucceeded, answer.Spec.Status, "status for %s", message)
	return answer
}
