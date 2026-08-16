# BUG_REPRO

The following failures were observed while validating the initial project state.
Each section records what failed, how to reproduce it, and the complete command output.
They are preserved intentionally; only failing build gates are omitted from the generated Dockerfile.

## Failure 1: Go test (.)

- Observed problem: `Go test (.)` failed in the initial project state.
- Working directory: `.`
- Command: `cd /app && GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test -count=1 ./...`
- Exit status: `1`

```text
?   	example.com/accounting-collab-drive/cmd/accounting-drive	[no test files]
--- FAIL: TestBatchReviewCreatesAnnotatedVersionsForAllDocuments (0.00s)
    service_test.go:17: batch review returned an error: review invoice-001: file handle limit reached: 3
FAIL
FAIL	example.com/accounting-collab-drive/internal/drive	0.001s
FAIL
```

## Architecture reproduction

### linux/amd64
- Go toolchain version: exit `0`
- Go build (.): exit `0`
- Go test (.): exit `1`
- Go run smoke (cmd/accounting-drive): exit `0`
### linux/arm64
- Go toolchain version: exit `0`
- Go build (.): exit `0`
- Go test (.): exit `1`
- Go run smoke (cmd/accounting-drive): exit `0`
