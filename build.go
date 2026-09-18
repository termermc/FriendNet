package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// === BEGIN BUILD TASKS ===

var tasks []taskSpec

func init() {
	tasks = []taskSpec{
		{
			Name: "pb-go",
			Desc: "Generates Go source files from protobuf schemas.",
			Cmd: cd("protocol",
				cmd("../tool/bin/buf", "lint"),
				cmd("../tool/bin/buf", "generate"),
			),
		},
		{
			Name: "pb-ts",
			Desc: "Generates TypeScript source files from protobuf schemas.\n" +
				"Expects all NPM dependencies to be installed.",
			Cmds: []cmdFn{
				cd("webui",
					cmd("npx", "buf", "lint"),
					cmd("npx", "buf", "generate"),
				),
				cd("server-widget",
					cmd("npx", "buf", "lint"),
					cmd("npx", "buf", "generate"),
				),
			},
		},
		{
			Name: "pb",
			Desc: "Generates source files from protobuf schemas.\n" +
				"Expects all NPM dependencies to be installed.",
			Deps: []string{
				"pb-go",
				"pb-ts",
			},
		},

		{
			Name: "adminui",
			Desc: "Builds the server admin UI.",
			Cmd: cd("adminui",
				cmd("go", "generate"),
			),
		},
		{
			Name: "server",
			Desc: "Builds the server.",
			Deps: []string{
				"adminui",
				"server-noui",
			},
		},
		{
			Name: "server-noui",
			Desc: "Builds the server (skips building the admin UI).\n" +
				"Optional args: [GOOS] [GOARCH]",
			Cmd: func(args []string) error {
				env := map[string]string{
					"CGO_ENABLED": "0",
					"GOOS":        "",
					"GOARCH":      "",
				}

				if len(args) > 0 {
					if len(args) < 2 {
						return fmt.Errorf(`must provide both GOOS and GOARCH, only provided GOOS`)
					}

					env["GOOS"] = args[0]
					env["GOARCH"] = args[1]
				}

				if err := os.MkdirAll("adminui/dist", 0755); err != nil {
					return err
				}

				return cd("server",
					cmdEnv(env,
						"go", "build",
						"-trimpath", "-ldflags=-s -w",
						"-o", "friendnet-server",
						"friendnet.org/server/cmd/server",
					),
				)(args)
			},
		},

		{
			Name: "webui",
			Desc: "Builds the client web UI.",
			Cmd: cd("webui",
				cmd("go", "generate"),
			),
		},
		{
			Name: "client",
			Desc: "Builds the client.",
			Deps: []string{
				"webui",
				"client-noui",
			},
		},
		{
			Name: "client-noui",
			Desc: "Builds the client (skips building the web UI).\n" +
				"Optional args: [GOOS] [GOARCH]",
			Cmd: func(args []string) error {
				env := map[string]string{
					"CGO_ENABLED": "0",
					"GOOS":        "",
					"GOARCH":      "",
				}

				if len(args) > 0 {
					if len(args) < 2 {
						return fmt.Errorf(`must provide both GOOS and GOARCH, only provided GOOS`)
					}

					env["GOOS"] = args[0]
					env["GOARCH"] = args[1]
				}

				if err := os.MkdirAll("webui/dist", 0755); err != nil {
					return err
				}

				outPath := "friendnet-client"
				var ldFlags string
				if env["GOOS"] == "windows" {
					ldFlags += "-H windowsgui"
					outPath += ".exe"
				}

				return cd("client",
					cmdEnv(env,
						"go", "build",
						"-trimpath", "-ldflags="+ldFlags,
						"-o", outPath,
						"friendnet.org/client/cmd/client",
					),
				)(args)
			},
		},

		{
			Name: "client-debs",
			Desc: "Builds Debian packages for the client.",
			Cmd: cd("packaging",
				cmd("node", "index.ts", "deb"),
			),
		},

		{
			Name: "client-debs-noui",
			Desc: "Builds Debian packages for the client (skips building the web UI).",
			Cmd: cd("packaging",
				cmd("node", "index.ts", "deb", "--no-ui"),
			),
		},

		{
			Name: "client-appimages",
			Desc: "Builds AppImage packages for the client.",
			Cmd: cd("packaging",
				cmd("node", "index.ts", "appimage"),
			),
		},
		{
			Name: "client-appimages-noui",
			Desc: "Builds AppImage packages for the client (skips building the web UI).",
			Cmd: cd("packaging",
				cmd("node", "index.ts", "appimage", "--no-ui"),
			),
		},

		{
			Name: "rpcclient",
			Desc: "Builds the server RPC client.\n" +
				"Optional args: [GOOS] [GOARCH]",
			Cmd: func(args []string) error {
				env := map[string]string{
					"CGO_ENABLED": "0",
					"GOOS":        "",
					"GOARCH":      "",
				}

				if len(args) > 0 {
					if len(args) < 2 {
						return fmt.Errorf(`must provide both GOOS and GOARCH, only provided GOOS`)
					}

					env["GOOS"] = args[0]
					env["GOARCH"] = args[1]
				}

				return cd("rpcclient",
					cmdEnv(env,
						"go", "build",
						"-trimpath", "-ldflags=-s -w",
						"-o", "friendnet-rpcclient",
						"friendnet.org/rpcclient/cmd/cli",
					),
				)(args)
			},
		},

		{
			Name: "run-rpcclient",
			Desc: `Runs the RPC client for development.\n` +
				`Assumes that the server RPC socket is in the "server" directory.`,
			Deps: []string{
				"rpcclient",
			},
			Cmd: cd("server",
				cmd("../rpcclient/friendnet-rpcclient"),
			),
		},

		{
			Name: "client-docker",
			Desc: "Builds the client Docker image." +
				"Optionally specify a tag to build it under.",
			Cmd: func(args []string) error {
				var tag string
				if len(args) > 0 {
					tag = args[0]
				} else {
					tag = "latest"
				}

				return cmd(
					"docker", "build",
					"-t", "git.termer.net/termer/friendnet-client:"+tag,
					"-f", "client.Dockerfile",
					".",
				)(args)
			},
		},
		{
			Name: "client-docker-publish",
			Desc: "Builds and publishes the client Docker image." +
				"Optionally specify a tag to publish it under.",
			Cmd: func(args []string) error {
				var tag string
				if len(args) > 0 {
					tag = args[0]
				} else {
					tag = "latest"
				}

				return cmd(
					"docker", "push",
					"git.termer.net/termer/friendnet-client:"+tag,
				)(args)
			},
		},

		{
			Name: "client-docker-dev",
			Desc: "Builds the client Docker image (dev tag).",
			Cmd:  task("client-docker", "dev"),
		},
		{
			Name: "client-docker-dev-publish",
			Desc: "Builds and publishes the client Docker image (dev tag).",
			Cmd:  task("client-docker-publish", "dev"),
		},

		{
			Name: "server-docker",
			Desc: "Builds the server Docker image." +
				"Optionally specify a tag to build it under.",
			Cmd: func(args []string) error {
				var tag string
				if len(args) > 0 {
					tag = args[0]
				} else {
					tag = "latest"
				}

				return cmd(
					"docker", "build",
					"-t", "git.termer.net/termer/friendnet-server:"+tag,
					"-f", "server.Dockerfile",
					".",
				)(args)
			},
		},
		{
			Name: "server-docker-publish",
			Desc: "Builds and publishes the server Docker image." +
				"Optionally specify a tag to publish it under.",
			Cmd: func(args []string) error {
				var tag string
				if len(args) > 0 {
					tag = args[0]
				} else {
					tag = "latest"
				}

				return cmd(
					"docker", "push",
					"git.termer.net/termer/friendnet-server:"+tag,
				)(args)
			},
		},

		{
			Name: "server-docker-dev",
			Desc: "Builds the server Docker image (dev tag).",
			Cmd:  task("server-docker", "dev"),
		},
		{
			Name: "server-docker-dev-publish",
			Desc: "Builds and publishes the server Docker image (dev tag).",
			Cmd:  task("server-docker-publish", "dev"),
		},

		{
			Name: "client-docker",
			Desc: "Builds the client Docker image." +
				"Optionally specify a tag to build it under.",
			Cmd: func(args []string) error {
				var tag string
				if len(args) > 0 {
					tag = args[0]
				} else {
					tag = "latest"
				}

				return cmd(
					"docker", "build",
					"-t", "git.termer.net/termer/friendnet-client:"+tag,
					"-f", "client.Dockerfile",
					".",
				)(args)
			},
		},

		{
			Name: "website",
			Desc: "Builds the website.",
			Cmd: cd("website",
				cmd("npm", "ci"),
				cmd("npm", "run", "build"),
			),
		},

		{
			Name: "release-artifacts",
			Desc: "Builds all release artifacts and places them in the release-artifacts directory.",
			Cmds: []cmdFn{
				rmrf("./release-artifacts"),
				mkdir("./release-artifacts"),
				task("webui"),
				task("client-noui", "windows", "amd64"),
				mv("client/friendnet-client.exe", "./release-artifacts/friendnet-client-windows_amd64.exe"),

				task("client-debs-noui"),
				mv("client/*.deb", "./release-artifacts/"),
				task("client-appimages-noui"),
				mv("client/*.AppImage", "./release-artifacts/"),

				task("adminui"),

				task("server-noui", "linux", "amd64"),
				mv("server/friendnet-server", "./release-artifacts/server"),
				task("rpcclient", "linux", "amd64"),
				mv("rpcclient/friendnet-rpcclient", "./release-artifacts/rpcclient"),
				chmod(0755, "./release-artifacts/server"),
				chmod(0755, "./release-artifacts/rpcclient"),
				cd("./release-artifacts",
					cmd("tar", "-czf", "friendnet-server-linux_amd64.tar.gz", "server", "rpcclient"),
					rmrf("server", "rpcclient"),
				),

				task("server-noui", "linux", "arm64"),
				mv("server/friendnet-server", "./release-artifacts/server"),
				task("rpcclient", "linux", "arm64"),
				mv("rpcclient/friendnet-rpcclient", "./release-artifacts/rpcclient"),
				chmod(0755, "./release-artifacts/server"),
				chmod(0755, "./release-artifacts/rpcclient"),
				cd("./release-artifacts",
					cmd("tar", "-czf", "friendnet-server-linux_arm64.tar.gz", "server", "rpcclient"),
					rmrf("server", "rpcclient"),
				),

				task("client-docker-publish"),
				task("client-docker-dev-publish"),

				task("server-docker-publish"),
				task("server-docker-dev-publish"),

				echo("Artifacts in ./release-artifacts, and new server Docker image pushed"),
			},
		},
	}
}

// === END BUILD TASKS ===

type cmdFn func(args []string) error

func echo(msg string) cmdFn {
	return func(_ []string) error {
		println(msg)
		return nil
	}
}
func chmod(mode os.FileMode, path string) cmdFn {
	return func(_ []string) error {
		return os.Chmod(path, mode)
	}
}
func mkdir(dir string) cmdFn {
	return func(_ []string) error {
		return os.MkdirAll(dir, 0755)
	}
}
func rmrf(pathGlobs ...string) cmdFn {
	return func(_ []string) error {
		for _, pathGlob := range pathGlobs {
			matches, err := filepath.Glob(pathGlob)
			if err != nil {
				return err
			}

			for _, match := range matches {
				if err = os.RemoveAll(match); err != nil {
					return err
				}
			}
		}

		return nil
	}
}
func mv(src string, dst string) cmdFn {
	return func(_ []string) error {
		srcMatches, err := filepath.Glob(src)
		if err != nil {
			return err
		}

		stat, err := os.Stat(dst)
		if err != nil && !os.IsNotExist(err) {
			return err
		}

		for _, match := range srcMatches {
			matchDst := dst
			if stat != nil && stat.IsDir() {
				matchDst = filepath.Join(dst, filepath.Base(match))
			}

			return os.Rename(match, matchDst)
		}

		return nil
	}
}
func and(fns ...cmdFn) cmdFn {
	return func(args []string) error {
		for _, fn := range fns {
			if err := fn(args); err != nil {
				return err
			}
		}
		return nil
	}
}
func cd(dir string, fns ...cmdFn) cmdFn {
	return func(args []string) error {
		if err := os.Chdir(dir); err != nil {
			return err
		}
		return and(fns...)(args)
	}
}
func cmd(exe string, args ...string) cmdFn {
	return cmdEnv(nil, exe, args...)
}
func cmdEnv(env map[string]string, exe string, args ...string) cmdFn {
	return func(_ []string) error {
		proc := exec.Command(exe, args...)

		proc.Env = os.Environ()
		if env != nil {
			envVals := make([]string, 0, len(env))
			for k, v := range env {
				envVals = append(envVals, k+"="+v)
			}
			proc.Env = slices.Concat(proc.Env, envVals)
		}
		proc.Stdin = os.Stdin
		proc.Stdout = os.Stdout
		proc.Stderr = os.Stderr

		return proc.Run()
	}
}
func sh(syntax string) cmdFn {
	return shEnv(nil, syntax)
}
func shEnv(env map[string]string, syntax string) cmdFn {
	return cmdEnv(env, "sh", "-c", syntax)
}
func task(name string, args ...string) cmdFn {
	return func(_ []string) error {
		spec, has := taskMap()[name]
		if !has {
			return fmt.Errorf(`tried to use undefined task %q`, name)
		}
		return spec.doCmds(args)
	}
}

type taskSpec struct {
	Name string
	Desc string
	Deps []string
	Cmd  cmdFn
	Cmds []cmdFn
}

func (s taskSpec) doCmds(args []string) error {
	cmds := make([]cmdFn, 0, len(s.Cmds)+1)
	if s.Cmd != nil {
		cmds = append(cmds, s.Cmd)
	}
	for _, c := range s.Cmds {
		cmds = append(cmds, c)
	}

	for _, c := range cmds {
		if err := os.Chdir(initCwd); err != nil {
			return err
		}
		if err := c(args); err != nil {
			return err
		}
	}

	return nil
}

var initCwd = func() string {
	cwd, err := os.Getwd()
	if err != nil {
		panic(fmt.Errorf(`failed to get cwd during startup: %w`, err))
	}
	return cwd
}()

var taskMap = sync.OnceValue(func() map[string]taskSpec {
	taskMap := make(map[string]taskSpec, len(tasks))

	for _, t := range tasks {
		_, has := taskMap[t.Name]
		if has {
			_, _ = fmt.Fprintf(os.Stderr, "build script bug: duplicate task %q\n", t.Name)
			os.Exit(1)
		}

		taskMap[t.Name] = t
	}

	return taskMap
})

func doSpec(spec taskSpec, taskMap map[string]taskSpec, args []string) error {
	for _, dep := range spec.Deps {
		depSpec, has := taskMap[dep]
		if !has {
			return fmt.Errorf(`task %q relies on nonexistent task %q`, spec.Name, dep)
		}

		if err := doSpec(depSpec, taskMap, nil); err != nil {
			return fmt.Errorf(`failed to run task %q: %w`, spec.Name, err)
		}
	}

	return spec.doCmds(args)
}

func printUsage() {
	println("Tasks:")
	for _, t := range tasks {
		name := t.Name
		const baseIndent = 4
		const suffix = ": "
		indent := baseIndent + len(name) + len(suffix)
		for range baseIndent {
			print(" ")
		}
		print(name + ": ")
		lns := strings.Split(t.Desc, "\n")
		println(lns[0])
		for _, ln := range lns[1:] {
			ln = strings.TrimSpace(ln)
			for range indent {
				print(" ")
			}
			println(ln)
		}
		println()
	}
}

func main() {
	args := os.Args[1:]

	if len(args) == 0 {
		printUsage()
		os.Exit(1)
		return
	}

	specName := args[0]
	spec, has := taskMap()[specName]
	if !has {
		_, _ = fmt.Fprintf(os.Stderr, "Unknown task %q\n", args[0])
		printUsage()
		os.Exit(1)
		return
	}

	if err := doSpec(spec, taskMap(), args[1:]); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, err.Error()+"\n")
		os.Exit(1)
	}
}
