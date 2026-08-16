package main

import (
	"bytes"
	"database/sql"
	"os/exec"
	"time"
)

// runScript executes script via /bin/sh -c, records the outcome in the runs
// table, and returns exit code + captured output. Runs execute synchronously;
// no job queue by design (see spec — fine at this scale).
func runScript(db *sql.DB, sourceType string, sourceID int64, script string) (exitCode int, stdout, stderr string) {
	started := time.Now()

	var cmd *exec.Cmd
	if hostExec() {
		// container must run with --pid=host + CAP_SYS_ADMIN/CAP_SYS_PTRACE (or --privileged)
		// so /proc/1 is the host's init and nsenter can join its namespaces.
		cmd = exec.Command("nsenter", "-t", "1", "-m", "-u", "-n", "-i", "--", "/bin/sh", "-c", script)
	} else {
		cmd = exec.Command("/bin/sh", "-c", script)
	}
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	exitCode = 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
			errBuf.WriteString("\n" + err.Error())
		}
	}
	stdout, stderr = outBuf.String(), errBuf.String()

	db.Exec(`INSERT INTO runs (source_type, source_id, started_at, exit_code, stdout, stderr) VALUES (?, ?, ?, ?, ?, ?)`,
		sourceType, sourceID, started, exitCode, stdout, stderr)

	return exitCode, stdout, stderr
}
