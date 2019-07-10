/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package trigger

import (
	"testing"

	"github.com/sirupsen/logrus"
	clienttesting "k8s.io/client-go/testing"

	"github.com/jenkins-x/lighthouse/pkg/prow/client/clientset/versioned/fake"
	"github.com/jenkins-x/lighthouse/pkg/prow/config"
	"github.com/jenkins-x/lighthouse/pkg/prow/github"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/diff"
	prowapi "k8s.io/test-infra/prow/apis/plumberJobs/v1"

	"github.com/jenkins-x/lighthouse/pkg/prow/fakegithub"
)

func TestCreateRefs(t *testing.T) {
	pe := *scm.PushHook{
		Ref: "refs/heads/master",
		Repo: scm.Repository{
			Owner: scm.User{
				Name: "kubernetes",
			},
			Name: "repo",
			Link: "https://example.com/kubernetes/repo",
		},
		After:   "abcdef",
		Compare: "https://example.com/kubernetes/repo/compare/abcdee...abcdef",
	}
	expected := builder.Refs{
		Org:      "kubernetes",
		Repo:     "repo",
		BaseRef:  "master",
		BaseSha:  "abcdef",
		BaseLink: "https://example.com/kubernetes/repo/compare/abcdee...abcdef",
	}
	if actual := createRefs(pe); !equality.Semantic.DeepEqual(expected, actual) {
		t.Errorf("diff between expected and actual refs:%s", diff.ObjectReflectDiff(expected, actual))
	}
}

func TestHandlePE(t *testing.T) {
	testCases := []struct {
		name      string
		pe        *scm.PushHook
		jobsToRun int
	}{
		{
			name: "branch deleted",
			pe: *scm.PushHook{
				Ref: "refs/heads/master",
				Repo: scm.Repository{
					FullName: "org/repo",
				},
				Deleted: true,
			},
			jobsToRun: 0,
		},
		{
			name: "no matching files",
			pe: *scm.PushHook{
				Ref: "refs/heads/master",
				Commits: []github.Commit{
					{
						Added: []string{"example.txt"},
					},
				},
				Repo: scm.Repository{
					FullName: "org/repo",
				},
			},
		},
		{
			name: "one matching file",
			pe: *scm.PushHook{
				Ref: "refs/heads/master",
				Commits: []github.Commit{
					{
						Added:    []string{"example.txt"},
						Modified: []string{"hack.sh"},
					},
				},
				Repo: scm.Repository{
					FullName: "org/repo",
				},
			},
			jobsToRun: 1,
		},
		{
			name: "no change matcher",
			pe: *scm.PushHook{
				Ref: "refs/heads/master",
				Commits: []github.Commit{
					{
						Added: []string{"example.txt"},
					},
				},
				Repo: scm.Repository{
					FullName: "org2/repo2",
				},
			},
			jobsToRun: 1,
		},
		{
			name: "branch name with a slash",
			pe: *scm.PushHook{
				Ref: "refs/heads/release/v1.14",
				Commits: []github.Commit{
					{
						Added: []string{"hack.sh"},
					},
				},
				Repo: scm.Repository{
					FullName: "org3/repo3",
				},
			},
			jobsToRun: 1,
		},
	}
	for _, tc := range testCases {
		g := &fakegithub.FakeClient{}
		fakePlumberClient := fake.NewSimpleClientset()
		c := Client{
			GitHubClient:  g,
			PlumberClient: fakePlumberClient.ProwV1().PlumberJobs("plumberJobs"),
			Config:        &config.Config{ProwConfig: config.ProwConfig{PlumberJobNamespace: "plumberJobs"}},
			Logger:        logrus.WithField("plugin", PluginName),
		}
		postsubmits := map[string][]config.Postsubmit{
			"org/repo": {
				{
					JobBase: config.JobBase{
						Name: "pass-butter",
					},
					RegexpChangeMatcher: config.RegexpChangeMatcher{
						RunIfChanged: "\\.sh$",
					},
				},
			},
			"org2/repo2": {
				{
					JobBase: config.JobBase{
						Name: "pass-salt",
					},
				},
			},
			"org3/repo3": {
				{
					JobBase: config.JobBase{
						Name: "pass-pepper",
					},
					Brancher: config.Brancher{
						Branches: []string{"release/v1.14"},
					},
				},
			},
		}
		if err := c.Config.SetPostsubmits(postsubmits); err != nil {
			t.Fatalf("failed to set postsubmits: %v", err)
		}
		err := handlePE(c, tc.pe)
		if err != nil {
			t.Errorf("test %q: handlePE returned unexpected error %v", tc.name, err)
		}
		var numStarted int
		for _, action := range fakePlumberClient.Fake.Actions() {
			switch action.(type) {
			case clienttesting.CreateActionImpl:
				numStarted++
			}
		}
		if numStarted != tc.jobsToRun {
			t.Errorf("test %q: expected %d jobs to run, got %d", tc.name, tc.jobsToRun, numStarted)
		}
	}
}