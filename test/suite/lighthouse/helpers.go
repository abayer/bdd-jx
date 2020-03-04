package lighthouse

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jenkins-x/bdd-jx/test/helpers"

	"github.com/jenkins-x/bdd-jx/test/utils"
	"github.com/jenkins-x/jx/pkg/gits"
	"github.com/jenkins-x/jx/pkg/util"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	defaultContext    = "pr-build"
	lhQuickstart      = "golang-http"
	brokenJenkinsXYml = `buildPack: go
pipelineConfig:
  pipelines:
    overrides:
      # Replace make-build on pullRequest in any stage/lifecycle with "exit 1" so the pipeline will fail.
      - pipeline: pullRequest
        name: make-linux
        step:
          sh: sleep 60 && exit 1
`
)

var _ = ChatOpsTests()

func ChatOpsTests() bool {
	return Describe("Lighthouse ChatOps", func() {
		var (
			T                helpers.TestOptions
			err              error
			provider         gits.GitProvider
			approverProvider gits.GitProvider
		)

		BeforeEach(func() {
			provider, err = T.GetGitProvider()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(provider).ShouldNot(BeNil())

			approverProvider, err = T.GetApproverGitProvider()
			Expect(err).ShouldNot(HaveOccurred())
			Expect(approverProvider).ShouldNot(BeNil())

			qsNameParts := strings.Split(lhQuickstart, "-")
			qsAbbr := ""
			for s := range qsNameParts {
				qsAbbr = qsAbbr + qsNameParts[s][:1]

			}
			applicationName := helpers.TempDirPrefix + qsAbbr + "-" + strconv.FormatInt(GinkgoRandomSeed(), 10)
			T = helpers.TestOptions{
				ApplicationName: applicationName,
				WorkDir:         helpers.WorkDir,
			}
			T.GitProviderURL()

			utils.LogInfof("Creating application %s in dir %s\n", util.ColorInfo(applicationName), util.ColorInfo(helpers.WorkDir))
		})

		Describe("Create a quickstart", func() {
			Context(fmt.Sprintf("by running jx create quickstart %s", lhQuickstart), func() {
				It("creates a new source repository", func() {
					args := []string{"create", "quickstart", "-b", "--org", T.GetGitOrganisation(), "-p", T.ApplicationName, "-f", lhQuickstart}

					gitProviderUrl, err := T.GitProviderURL()
					Expect(err).NotTo(HaveOccurred())
					if gitProviderUrl != "" {
						utils.LogInfof("Using Git provider URL %s\n", gitProviderUrl)
						args = append(args, "--git-provider-url", gitProviderUrl)
					}
					argsStr := strings.Join(args, " ")
					By(fmt.Sprintf("calling jx %s", argsStr), func() {
						T.ExpectJxExecution(T.WorkDir, helpers.TimeoutSessionWait, 0, args...)
					})

					applicationName := T.GetApplicationName()
					owner := T.GetGitOrganisation()

					By("adding the approver to OWNERS", func() {
						createdPR := T.CreatePullRequestWithLocalChange(fmt.Sprintf("Adding %s to OWNERS", helpers.PRApproverUsername), func(workDir string) {
							// overwrite the existing OWNERS with a new one containing the approver user
							fileName := "OWNERS"
							owners := filepath.Join(workDir, fileName)

							data := []byte(fmt.Sprintf("approvers:\n- %s\n- %s\nreviewers:\n- %s\n- %s\n",
								provider.UserAuth().Username, helpers.PRApproverUsername,
								provider.UserAuth().Username, helpers.PRApproverUsername))
							err := ioutil.WriteFile(owners, data, util.DefaultWritePermissions)
							if err != nil {
								panic(err)
							}

							T.ExpectCommandExecution(workDir, time.Minute, 0, "git", "add", fileName)
						})

						ownersPR, err := T.GetPullRequestByNumber(provider, createdPR.Owner, createdPR.Repository, createdPR.PullRequestNumber)
						Expect(err).NotTo(HaveOccurred())
						Expect(ownersPR).ShouldNot(BeNil())

						By("merging the OWNERS PR")
						err = provider.MergePullRequest(ownersPR, "PR merge")
						Expect(err).ShouldNot(HaveOccurred())

						T.WaitForPRToMerge(provider, ownersPR.Owner, ownersPR.Repo, *ownersPR.Number, ownersPR.URL)
					})

					prTitle := "My First PR commit"
					var pr *gits.GitPullRequest
					By("performing a pull request on the source and making sure it fails", func() {
						createdPR := T.CreatePullRequestWithLocalChange(prTitle, func(workDir string) {
							// overwrite the existing jenkins-x.yml with a failing one
							fileName := "jenkins-x.yml"
							jxYml := filepath.Join(workDir, fileName)

							data := []byte(brokenJenkinsXYml)
							err := ioutil.WriteFile(jxYml, data, util.DefaultWritePermissions)
							if err != nil {
								panic(err)
							}

							T.ExpectCommandExecution(workDir, time.Minute, 0, "git", "add", fileName)
						})

						pr, err = T.GetPullRequestByNumber(provider, createdPR.Owner, createdPR.Repository, createdPR.PullRequestNumber)
						Expect(err).NotTo(HaveOccurred())
						Expect(pr).ShouldNot(BeNil())

						T.WaitForPullRequestCommitStatus(provider, pr, "failure", []string{defaultContext})
					})

					By("approving pull request", func() {
						err = T.ApprovePR(provider, approverProvider, pr)
						Expect(err).ShouldNot(HaveOccurred())
					})

					By("retest failed context with it failing again", func() {
						err = approverProvider.AddPRComment(pr, "/retest")
						Expect(err).ShouldNot(HaveOccurred())

						// Wait until we see a pending status, meaning we've got a new build
						T.WaitForPullRequestCommitStatus(provider, pr, "pending", []string{defaultContext})

						// Wait until we see the build fail.
						T.WaitForPullRequestCommitStatus(provider, pr, "failure", []string{defaultContext})
					})

					By("override failed context, see status as success, wait for it to merge", func() {
						err = approverProvider.AddPRComment(pr, fmt.Sprintf("/override %s", defaultContext))
						Expect(err).ShouldNot(HaveOccurred())

						// Wait until we see a success status
						T.WaitForPullRequestCommitStatus(provider, pr, "success", []string{defaultContext})

						T.WaitForPRToMerge(provider, pr.Owner, pr.Repo, *pr.Number, pr.URL)
					})

					// TODO: Later: add multiple contexts, one more required, one more optional

					By("creating an issue and assigning it to a valid user", func() {
						issue := &gits.GitIssue{
							Owner: owner,
							Repo:  applicationName,
							Title: "Test the /assign command",
							Body:  "This tests assigning a user using a ChatOps command",
						}
						err = T.CreateIssueAndAssignToUserWithChatOpsCommand(issue, provider)
						Expect(err).NotTo(HaveOccurred())
					})

					if T.DeleteApplications() {
						args = []string{"delete", "application", "-b", T.ApplicationName}
						argsStr := strings.Join(args, " ")
						By(fmt.Sprintf("calling %s to delete the application", argsStr), func() {
							T.ExpectJxExecution(T.WorkDir, helpers.TimeoutSessionWait, 0, args...)
						})
					}

					if T.DeleteRepos() {
						args = []string{"delete", "repo", "-b", "--github", "-o", T.GetGitOrganisation(), "-n", T.ApplicationName}
						argsStr = strings.Join(args, " ")

						By(fmt.Sprintf("calling %s to delete the repository", os.Args), func() {
							T.ExpectJxExecution(T.WorkDir, helpers.TimeoutSessionWait, 0, args...)
						})
					}
				})
			})
		})
	})
}
