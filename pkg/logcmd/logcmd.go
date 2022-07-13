package logcmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/sirupsen/logrus"
)

func logReader(ctx context.Context, file *os.File, log *logrus.Logger, prefix string, level logrus.Level) error {

	if ctx != nil {
		if deadline, ok := ctx.Deadline(); ok {
			if err := file.SetDeadline(deadline); err != nil {
				log.Warnf("ctx deadline (%v) will not be respected", deadline)
			}
		}
	}

	rd := bufio.NewReader(file)
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			if os.IsTimeout(err) {
				return fmt.Errorf("read timeout due to context: %w", ctx.Err())
			}
			return err
		}

		log.Logf(level, "%s%s", prefix, line)
	}
}

func runAndLogCommand(
	ctx context.Context,
	cmd *exec.Cmd,
	log *logrus.Logger,
	stdoutLevel, stderrLevel logrus.Level,
) error {

	// prepare pipes
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("StdErrPipe() failed: %w", err)
	}
	defer stderr.Close()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("StdOutPipe() failed: %w", err)
	}
	defer stdout.Close()

	// start command
	log.WithField("path", cmd.Path).WithField("args", cmd.Args).Info("starting command")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to execute command: %w", err)
	}

	// cmd.StderrPipe() and cmd.StdoutPipe() docs say that we need to wait for the pipe reads to
	// finish, before waiting for the command using cmd.Wait(). However, if the command was
	// created with a timeout ctx, then the process will be killed only in cmd.Wait().
	//
	// to solve this problem, rsc suggests to use os.Pipe() and SetReadDeadline:
	// https://github.com/golang/go/issues/21922#issuecomment-338792340
	//
	// I was not sure how the pipes would be closed on the child end, but it seems that's taken
	// care by go itself:
	//  - https://github.com/golang/go/blob/bf2ef26be3593d24487311576d85ec601185fbf4/src/os/pipe_unix.go#L13-L28
	//  - https://github.com/golang/go/blob/bf2ef26be3593d24487311576d85ec601185fbf4/src/syscall/exec_unix.go#L19-L65
	//
	// Because I'm lazy, howerver, I'll just reuse the file descriptors from
	// cmd.Std{err,out}Pipe, since they are also calling os.Pipe():
	// - https://github.com/golang/go/blob/bf2ef26be3593d24487311576d85ec601185fbf4/src/os/exec/exec.go#L786
	// - https://github.com/golang/go/blob/bf2ef26be3593d24487311576d85ec601185fbf4/src/os/exec/exec.go#L761
	stderrFile := stderr.(*os.File)
	stdoutFile := stdout.(*os.File)

	// start logging
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		err = logReader(ctx, stdoutFile, log, "stdout> ", stdoutLevel)
		if err != nil {
			log.Warnf("failed to read from stdout: %v", err)
		}
	}()

	go func() {
		defer wg.Done()
		err = logReader(ctx, stderrFile, log, "stderr> ", stderrLevel)
		if err != nil {
			log.Warnf("failed to read from stderr: %v", err)
		}
	}()

	// we need to wait for the pipes before waiting for the command
	// see: https://pkg.go.dev/os/exec#Cmd.StdoutPipe
	wg.Wait()

	ret := cmd.Wait()
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return ret
}

func RunAndLogCommand(
	cmd *exec.Cmd,
	log *logrus.Logger,
) error {
	return runAndLogCommand(nil, cmd, log, logrus.InfoLevel, logrus.WarnLevel)
}

func RunAndLogCommandContext(
	ctx context.Context,
	log *logrus.Logger,
	cmd0 string,
	cmdArgs ...string,
) error {
	cmd := exec.CommandContext(ctx, cmd0, cmdArgs...)
	return runAndLogCommand(ctx, cmd, log, logrus.InfoLevel, logrus.WarnLevel)
}
