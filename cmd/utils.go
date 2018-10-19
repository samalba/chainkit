package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/acarl005/stripansi"
	"github.com/blocklayerhq/chainkit/pkg/ui"
	"github.com/schollz/progressbar"
	"github.com/spf13/cobra"
)

func getCwd(cmd *cobra.Command) string {
	cwd, err := cmd.Flags().GetString("cwd")
	if err != nil {
		ui.Fatal("unable to resolve --cwd: %v", err)
		return ""
	}
	if cwd == "" {
		cwd, err = os.Getwd()
		if err != nil {
			ui.Fatal("unable to determine current directory: %v", err)
			return ""
		}
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		ui.Fatal("unable to parse %q: %v", cwd, err)
	}
	return abs
}

func goPath() string {
	p := os.Getenv("GOPATH")
	if p != "" {
		return p
	}
	return path.Join(os.Getenv("HOME"), "go")
}

func goSrc() string {
	return path.Join(goPath(), "src")
}

func dockerRun(rootDir, name string, args ...string) error {
	dataDir := path.Join(rootDir, "data")

	daemonName := name + "d"
	cliName := name + "cli"

	// -v "${data_dir}/${APP_NAME}d:/root/.${APP_NAME}d"
	daemonDir := path.Join(dataDir, daemonName)
	daemonDirContainer := path.Join("/", "root", "."+daemonName)

	// -v "${data_dir}/${APP_NAME}cli:/root/.${APP_NAME}cli"
	cliDir := path.Join(dataDir, cliName)
	cliDirContainer := path.Join("/", "root", "."+cliName)

	cmd := []string{
		"run", "--rm",
		"-p", "26656:26656",
		"-p", "26657:26657",
		"-v", daemonDir + ":" + daemonDirContainer,
		"-v", cliDir + ":" + cliDirContainer,
		name + ":latest",
		daemonName,
	}
	cmd = append(cmd, args...)

	return docker(rootDir, cmd...)
}

func docker(rootDir string, args ...string) error {
	return run(rootDir, "docker", args...)
}

func run(rootDir, command string, args ...string) error {
	ui.Verbose("$ %s %s", command, strings.Join(args, " "))
	cmd := exec.Command(command)
	cmd.Args = append([]string{command}, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = rootDir
	return cmd.Run()
}

func dockerBuild(rootDir, name string, verbose bool) error {
	cmd := exec.Command("docker", "build", "-t", name, rootDir)
	outReader, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	defer outReader.Close()
	errReader, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	defer errReader.Close()

	cmdReader := io.MultiReader(outReader, errReader)

	scanner := bufio.NewScanner(cmdReader)
	go func() {
		var (
			progress *progressbar.ProgressBar
		)
		for scanner.Scan() {
			text := stripansi.Strip(scanner.Text())

			// Dockerfile step
			if strings.HasPrefix(text, "Step ") {
				switch {
				case strings.Contains(text, "RUN apk add --no-cache"):
					fmt.Println(ui.Small("[1/4]"), "📦 Setting up the build environment...")
				case strings.Contains(text, "RUN dep ensure"):
					fmt.Println(ui.Small("[2/4]"), "🔎 Fetching dependencies...")
				case strings.Contains(text, "RUN find vendor"):
					fmt.Println(ui.Small("[3/4]"), "🔗 Installing dependencies...")
				case strings.Contains(text, "RUN     CGO_ENABLED=0 go build"):
					fmt.Println(ui.Small("[4/4]"), "🔨 Compiling application...")
				}
			}

			// Progress bars
			if !verbose {
				var (
					step  int
					total int
				)
				sr := strings.NewReader(text)
				if n, _ := fmt.Fscanf(sr, "(%d/%d) Wrote", &step, &total); n == 2 {
					if progress == nil {
						progress = progressbar.NewOptions(
							total,
							progressbar.OptionSetTheme(progressbar.Theme{
								Saucer:        "#",
								SaucerPadding: "-",
								BarStart:      "[",
								BarEnd:        "]",
							}),
						)
					}
					progress.Add(1)
					if step == total {
						progress.Finish()
						progress.Clear()
						progress = nil
					}
				}
			}

			if verbose {
				ui.Verbose(text)
			}
		}
	}()
	err = cmd.Start()
	if err != nil {
		return err
	}

	err = cmd.Wait()
	if err != nil {
		return err
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}