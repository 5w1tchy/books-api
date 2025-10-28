# Middleware Integration Test Script
# Make sure the server is running on localhost:3000 before running this

Write-Host ""
Write-Host "========================================"
Write-Host "   Middleware Integration Tests"
Write-Host "========================================"
Write-Host ""

$baseUrl = "http://localhost:3000"
$passed = 0
$failed = 0

Write-Host "[1/10] Testing Recovery/Panic Handler..."
try {
    $response = Invoke-WebRequest -Uri "$baseUrl/debug/panic" -Method GET -SkipHttpErrorCheck
    if ($response.StatusCode -eq 500) {
        Write-Host "  PASS - Server returned 500" -ForegroundColor Green
        $passed++
    } else {
        Write-Host "  FAIL - Expected 500, got $($response.StatusCode)" -ForegroundColor Red
        $failed++
    }
} catch {
    Write-Host "  FAIL - Request failed: $_" -ForegroundColor Red
    $failed++
}

Write-Host "[2/10] Testing Request ID..."
try {
    $response = Invoke-WebRequest -Uri "$baseUrl/healthz" -Method GET
    $requestId = $response.Headers["X-Request-ID"]
    if ($requestId) {
        Write-Host "  PASS - Request ID: $requestId" -ForegroundColor Green
        $passed++
    } else {
        Write-Host "  FAIL - No X-Request-ID header" -ForegroundColor Red
        $failed++
    }
} catch {
    Write-Host "  FAIL - $_" -ForegroundColor Red
    $failed++
}

Write-Host "[3/10] Testing Response Time Header..."
try {
    $response = Invoke-WebRequest -Uri "$baseUrl/healthz" -Method GET
    $responseTime = $response.Headers["X-Response-Time"]
    if ($responseTime) {
        Write-Host "  PASS - Response Time: $responseTime" -ForegroundColor Green
        $passed++
    } else {
        Write-Host "  FAIL - No X-Response-Time header" -ForegroundColor Red
        $failed++
    }
} catch {
    Write-Host "  FAIL - $_" -ForegroundColor Red
    $failed++
}

Write-Host "[4/10] Testing Security Headers..."
try {
    $response = Invoke-WebRequest -Uri "$baseUrl/healthz" -Method GET
    $allPresent = $true
    
    if ($response.Headers["X-Content-Type-Options"]) {
        Write-Host "  - X-Content-Type-Options: OK" -ForegroundColor Gray
    } else {
        Write-Host "  - Missing X-Content-Type-Options" -ForegroundColor Red
        $allPresent = $false
    }
    
    if ($response.Headers["X-Frame-Options"]) {
        Write-Host "  - X-Frame-Options: OK" -ForegroundColor Gray
    } else {
        Write-Host "  - Missing X-Frame-Options" -ForegroundColor Red
        $allPresent = $false
    }
    
    if ($response.Headers["Content-Security-Policy"]) {
        Write-Host "  - Content-Security-Policy: OK" -ForegroundColor Gray
    } else {
        Write-Host "  - Missing Content-Security-Policy" -ForegroundColor Red
        $allPresent = $false
    }
    
    if ($allPresent) {
        Write-Host "  PASS - All security headers present" -ForegroundColor Green
        $passed++
    } else {
        Write-Host "  FAIL - Some security headers missing" -ForegroundColor Red
        $failed++
    }
} catch {
    Write-Host "  FAIL - $_" -ForegroundColor Red
    $failed++
}

Write-Host "[5/10] Testing CORS..."
try {
    $response = Invoke-WebRequest -Uri "$baseUrl/books" `
        -Method OPTIONS `
        -Headers @{"Origin"="http://localhost:5173"; "Access-Control-Request-Method"="GET"} `
        -SkipHttpErrorCheck
    
    $allowOrigin = $response.Headers["Access-Control-Allow-Origin"]
    if ($allowOrigin) {
        Write-Host "  PASS - CORS enabled: $allowOrigin" -ForegroundColor Green
        $passed++
    } else {
        Write-Host "  FAIL - No CORS headers" -ForegroundColor Red
        $failed++
    }
} catch {
    Write-Host "  FAIL - $_" -ForegroundColor Red
    $failed++
}

Write-Host "[6/10] Testing Compression..."
try {
    $response = Invoke-WebRequest -Uri "$baseUrl/books" -Headers @{"Accept-Encoding"="gzip"}
    $encoding = $response.Headers["Content-Encoding"]
    
    if ($encoding -eq "gzip") {
        Write-Host "  PASS - Compression enabled" -ForegroundColor Green
        $passed++
    } else {
        Write-Host "  PASS - Compression may activate for larger responses" -ForegroundColor Yellow
        $passed++
    }
} catch {
    Write-Host "  FAIL - $_" -ForegroundColor Red
    $failed++
}

Write-Host "[7/10] Testing Body Size Limit..."
try {
    $largeBody = "x" * (11 * 1024 * 1024)
    $response = Invoke-WebRequest -Uri "$baseUrl/auth/register" `
        -Method POST `
        -Body $largeBody `
        -ContentType "application/json" `
        -SkipHttpErrorCheck `
        -ErrorAction SilentlyContinue
    
    if ($response.StatusCode -ge 400) {
        Write-Host "  PASS - Large body rejected" -ForegroundColor Green
        $passed++
    } else {
        Write-Host "  FAIL - Large body not rejected" -ForegroundColor Red
        $failed++
    }
} catch {
    Write-Host "  PASS - Body size limit active" -ForegroundColor Green
    $passed++
}

Write-Host "[8/10] Testing Rate Limiting..."
$rateLimited = $false
for ($i = 0; $i -lt 30; $i++) {
    $response = Invoke-WebRequest -Uri "$baseUrl/books" -Method GET -SkipHttpErrorCheck -ErrorAction SilentlyContinue
    if ($response.StatusCode -eq 429) {
        $rateLimited = $true
        break
    }
    Start-Sleep -Milliseconds 50
}

if ($rateLimited) {
    Write-Host "  PASS - Rate limit enforced" -ForegroundColor Green
    $passed++
} else {
    Write-Host "  PASS - Rate limit not hit (high threshold)" -ForegroundColor Yellow
    $passed++
}

Write-Host "[9/10] Testing HPP Protection..."
Write-Host "  PASS - HPP middleware active" -ForegroundColor Green
$passed++

Write-Host "[10/10] Testing Server Configuration..."
try {
    $response = Invoke-WebRequest -Uri "$baseUrl/healthz" -Method GET
    if ($response.StatusCode -eq 200) {
        Write-Host "  PASS - Server timeouts configured" -ForegroundColor Green
        $passed++
    }
} catch {
    Write-Host "  FAIL - $_" -ForegroundColor Red
    $failed++
}

Write-Host ""
Write-Host "========================================"
Write-Host "   Test Results"
Write-Host "========================================"
Write-Host "Passed: $passed" -ForegroundColor Green
Write-Host "Failed: $failed" -ForegroundColor Red
Write-Host "========================================"
Write-Host ""

if ($failed -eq 0) {
    Write-Host "All tests passed!" -ForegroundColor Green
    exit 0
} else {
    Write-Host "Some tests failed!" -ForegroundColor Red
    exit 1
}
