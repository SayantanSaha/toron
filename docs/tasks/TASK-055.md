# TASK-055: Implement Centralized Version Tracking

## Task Details
- **Requirement**: REQ-055
- **Status**: IN_PROGRESS

## Tasks
1. Create canonical root `VERSION` file (`1.5.0`).
2. Implement `pkg/version/version.go` with variables `Version`, `GitCommit`, `BuildDate`, `Info()`, and unit tests (`version_test.go`).
3. Wire `pkg/version` into `cmd/toron/main.go` supporting `-v` and `-version` CLI flags and startup logs.
4. Update `pkg/server/internal_api.go` to consume `version.Version` dynamically.
5. Update `Makefile` with `VERSION`, `GIT_COMMIT`, `BUILD_DATE` and `-ldflags` compiler flags for all target architectures.
6. Update `install.sh` and `install.bat` to read dynamically from `VERSION`.
7. Verify all test suites and build outputs.
