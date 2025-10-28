# Middleware Tests

This directory contains unit tests for all API middlewares.

## Running Tests

```bash
# Run all middleware tests
go test ./test/middlewares/... -v

# Run specific test
go test ./test/middlewares/... -run TestRecovery -v

# Run with coverage
go test ./test/middlewares/... -cover
```

## Test Files

- `recovery_test.go` - Tests panic recovery middleware
- `body_size_limit_test.go` - Tests request body size limiting
- `request_id_test.go` - Tests request ID generation and propagation
- `response_time_test.go` - Tests response time header injection
- `security_headers_test.go` - Tests security header injection (CSP, X-Frame-Options, etc.)

## Integration Testing

For manual integration testing, use the PowerShell script in the root directory:

```powershell
.\test-middlewares.ps1
```

**Note:** Server must be running on `localhost:3000` before running integration tests.
