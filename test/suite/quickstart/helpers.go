package quickstart

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/jenkins-x/bdd-jx/test/helpers"

	"github.com/jenkins-x/bdd-jx/test/utils"
	"github.com/jenkins-x/jx/pkg/gits"
	"github.com/jenkins-x/jx/pkg/util"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var (
	IncludedQuickstarts = []string{"node-http", "spring-boot-http-gradle", "golang-http"}
	_                   = AllQuickstartsTest()
)

// AllQuickstartsTest is responsible for running `jx get quickstarts, and creating a test for each quickstart currnetly
// available
// Individual tests can be run with `go test test/quickstart -ginkgo.focus <quickstart name>`
func AllQuickstartsTest() []bool {
	cmd := exec.Command("jx", "get", "quickstarts")
	bytes, err := cmd.CombinedOutput()
	if err != nil {
		panic(errors.Wrapf(err, "running jx get quickstarts, output was %s", string(bytes)))
	}
	if err != nil {
		panic(errors.WithStack(err))
	}
	tests := make([]bool, 0)
	for _, testQuickstartName := range IncludedQuickstarts {
		tests = append(tests, CreateQuickstartsTests(testQuickstartName))
	}
	return tests
}

//CreateQuickstartsTests creates a batch quickstart test for the given quickstart
func CreateQuickstartsTests(quickstartName string) bool {
	return createQuickstartTests(quickstartName)
}

// CreateQuickstartTest Creates quickstart tests.
func createQuickstartTests(quickstartName string) bool {
	return Describe("quickstart "+quickstartName+"\n", func() {
		var T helpers.TestOptions

		BeforeEach(func() {
			qsNameParts := strings.Split(quickstartName, "-")
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
			Context(fmt.Sprintf("by running jx create quickstart %s", quickstartName), func() {
				It("creates a new source repository and promotes it to staging", func() {
					args := []string{"create", "quickstart", "-b", "--org", T.GetGitOrganisation(), "-p", T.ApplicationName, "-f", quickstartName}

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

					if T.WeShouldTestChatOpsCommands() {
						gitProvider, err := T.GetGitProvider()
						Expect(err).NotTo(HaveOccurred())
						By("creating an issue and assigning it to a valid user", func() {
							issue := &gits.GitIssue{
								Owner: owner,
								Repo:  applicationName,
								Title: "Test the /assign command",
								Body:  "This tests assigning a user using a ChatOps command",
							}
							err = T.CreateIssueAndAssignToUserWithChatOpsCommand(issue, gitProvider)
							Expect(err).NotTo(HaveOccurred())
						})

						By("attempting to LGTM our own PR", func() {
							err = T.AttemptToLGTMOwnPR(gitProvider, owner, applicationName)
							Expect(err).NotTo(HaveOccurred())
						})

						By("adding a hold label", func() {
							err = T.AddHoldLabelToPRWithChatOpsCommand(gitProvider, owner, applicationName)
							Expect(err).NotTo(HaveOccurred())
						})

						By("adding a WIP label", func() {
							err = T.AddWIPLabelToPRByUpdatingTitle(gitProvider, owner, applicationName)
							Expect(err).NotTo(HaveOccurred())
						})
					}
				})
			})
		})
		Describe("Create a quickstart with invalid parameters", func() {
			Context("when -p param (project name) is missing", func() {
				It("exits with signal 1", func() {
					args := []string{"create", "quickstart", "-b", "--org", T.GetGitOrganisation(), "-f", quickstartName}
					argsStr := strings.Join(args, " ")
					By(fmt.Sprintf("calling jx %s", argsStr), func() {
						T.ExpectJxExecution(T.WorkDir, helpers.TimeoutSessionWait, 1, args...)
					})
				})
			})
			Context("when -f param (filter) does not match any quickstart", func() {
				It("exits with signal 1", func() {
					args := []string{"create", "quickstart", "-b", "--org", T.GetGitOrganisation(), "-p", T.ApplicationName, "-f", "the_derek_zoolander_app_for_being_really_really_good_looking"}
					argsStr := strings.Join(args, " ")
					By(fmt.Sprintf("calling jx %s", argsStr), func() {
						T.ExpectJxExecution(T.WorkDir, helpers.TimeoutSessionWait, 1, args...)
					})
				})
			})
		})
	})
}
