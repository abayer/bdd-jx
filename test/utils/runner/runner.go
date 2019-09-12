package runner

import (
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"

	"github.com/jenkins-x/bdd-jx/test/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

const (
	jx = "jx"
)

var (
	// jxRunner session timeout
	TimeoutJxRunner     = utils.GetTimeoutFromEnv("BDD_TIMEOUT_JX_RUNNER", 5)
	coverageOutputRegex = regexp.MustCompile(`(?m:(PASS|FAIL)\n\s*coverage: ([\d\.]*%) of statements in [\w\.\/]*\n)`)
)

// Runner runs a jx command
type JxRunner struct {
	cwd      string
	timeout  time.Duration
	exitCode int
}

// New creates a new jx command runnner
func New(cwd string, timeout *time.Duration, exitCode int) *JxRunner {
	if timeout == nil {
		timeout = &TimeoutJxRunner
	}
	return &JxRunner{
		cwd:      cwd,
		timeout:  *timeout,
		exitCode: exitCode,
	}
}

// Run runs a jx command
func (r *JxRunner) Run(args ...string) {
	err := r.run(GinkgoWriter, GinkgoWriter, args...)
	utils.ExpectNoError(err)
}

func (r *JxRunner) run(out io.Writer, errOut io.Writer, args ...string) error {
	if testing.Verbose() {
		utils.LogInfof("\033[1mRUNNER:\033[0mAbout to execute jx %s in %s with timeout %v expecting exit code %d\n", strings.Join(args, " "), r.cwd, r.timeout, r.exitCode)
	}
	command := exec.Command(jx, args...)
	command.Dir = r.cwd
	session, err := gexec.Start(command, out, errOut)
	if err != nil {
		return errors.WithStack(err)
	}
	session.Wait(r.timeout)
	Eventually(session).Should(gexec.Exit())
	if testing.Verbose() {
		utils.LogInfof("\033[1mRUNNER:\033[0mExecution completed with exit code %d\n", session.ExitCode())
	}
	if session.ExitCode() != r.exitCode {
		return errors.Errorf("expected exit code %d but got %d whilst running command %s %s", r.exitCode, session.ExitCode(), jx, strings.Join(args, " "))
	}
	return nil
}

// Run runs a jx command
func (r *JxRunner) RunWithOutput(args ...string) (string, error) {
	rOut, out, err := os.Pipe()
	if err != nil {
		return "", errors.WithStack(err)
	}

	// combine out and errOut
	rErr := r.run(out, out, args...)
	err = out.Close()
	if err != nil {
		return "", errors.WithStack(err)
	}
	outBytes, err := ioutil.ReadAll(rOut)
	if err != nil {
		return "", errors.WithStack(err)
	}
	err = rOut.Close()
	if err != nil {
		return "", errors.WithStack(err)
	}
	answer := string(outBytes)
	if rErr != nil {
		return "", errors.Wrapf(err, "running jx %s output %s", strings.Join(args, " "), answer)
	}
	return strings.TrimSpace(RemoveCoverageText(answer, args...)), nil
}

func RemoveCoverageText(s string, args ...string) string {
	coverageOutput := coverageOutputRegex.FindStringSubmatch(s)
	if len(coverageOutput) == 3 {
		utils.LogInfof("when running %s %s coverage was %s\n", jx, strings.Join(args, " "), coverageOutput[2])
	}
	answer := coverageOutputRegex.ReplaceAllString(s, "")
	return strings.TrimSpace(answer)
}
