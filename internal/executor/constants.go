package executor

const (
	// taskQueueBufferMultiplier sizes the task queue buffer relative to the
	// number of workers, giving the queue some slack for smooth operation.
	taskQueueBufferMultiplier = 2

	// executionIDShortLen is the number of leading characters of an execution
	// ID used when constructing and matching short, human-readable log file
	// names.
	executionIDShortLen = 8

	// scannerInitialBufferSize is the initial buffer size for bufio.Scanner
	// instances that read process output or log files line by line.
	scannerInitialBufferSize = 64 * 1024

	// scannerMaxLineSize is the maximum line size a bufio.Scanner will
	// buffer (1MB), preventing unbounded memory growth on pathological
	// output.
	scannerMaxLineSize = 1024 * 1024

	// broadcastReaderCount is the number of goroutines started to stream
	// stdout/stderr to the log broadcaster in real time.
	broadcastReaderCount = 2

	// logTimestampFormat is the timestamp layout used within log file lines.
	logTimestampFormat = "2006-01-02 15:04:05"

	// logFileNameTimeFormat is the timestamp layout used when naming log files.
	logFileNameTimeFormat = "2006-01-02_15-04-05"

	// logDirPermissions restricts task log directories to owner/group access.
	logDirPermissions = 0o750

	// logSeparator visually separates sections within a written log file.
	logSeparator = "─────────────────────────────────────────────"
)
