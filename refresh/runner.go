package refresh

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

func (m *Manager) runner() {
	var cmd *exec.Cmd
	for {
		<-m.Restart
		if cmd != nil {
			// kill the previous command
			pid := cmd.Process.Pid
			m.Logger.Success("Stopping: PID %d", pid)
			cmd.Process.Kill()
		}
		if m.Debug {
			bp := m.FullBuildPath()
			args := []string{"exec", bp}
			args = append(args, m.CommandFlags...)
			cmd = exec.Command("dlv", args...)
		} else {
			cmd = exec.Command(m.FullBuildPath(), m.CommandFlags...)
		}
		go func() {
			err := m.runAndListen(cmd)
			if err != nil {
				m.Logger.Error(err)
			}
		}()
	}
}

func (m *Manager) runAndListen(cmd *exec.Cmd) error {
	cmd.Stderr = m.Stderr
	if cmd.Stderr == nil {
		cmd.Stderr = os.Stderr
	}

	cmd.Stdin = m.Stdin
	if cmd.Stdin == nil {
		cmd.Stdin = os.Stdin
	}

	cmd.Stdout = m.Stdout
	if cmd.Stdout == nil {
		cmd.Stdout = os.Stdout
	}

	var (
		stderr bytes.Buffer
	)
	cmd.Stderr = io.MultiWriter(&stderr, cmd.Stderr)

	// Set the environment variables from config
	if len(m.CommandEnv) != 0 {
		cmd.Env = append(m.CommandEnv, os.Environ()...)
	}

	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("%s\n%s", err, stderr.String())
	}

	m.Logger.Success("Main Running: %s (PID: %d)", strings.Join(cmd.Args, " "), cmd.Process.Pid)

	err = cmd.Wait()
	if err != nil {
		return fmt.Errorf("%s\n%s", err, stderr.String())
	}
	return nil
}

func (m *Manager) runTasks() error {
	var taskCmds []*exec.Cmd
	if len(m.Livereload.Tasks) != 0 {
		for _, v := range m.Livereload.Tasks {
			ss := strings.Split(v, " ")
			taskCmds = append(taskCmds, exec.Command(ss[0], ss[1:]...))
		}
		cmd := taskCmds[0]
		err := cmd.Start()
		if err != nil {
			return err
		}
		m.Logger.Success("Task Running: %s", strings.Join(cmd.Args, " "))

		for _, v := range taskCmds[1:] {
			pipe, err := v.StdoutPipe()
			if err != nil {
				return err
			}
			m.Logger.Success("Task Running: %s", strings.Join(v.Args, " "))
			cmd.Stdin = pipe
		}

		if err := cmd.Wait(); err != nil {
			return err
		}
	}
	return nil
}
