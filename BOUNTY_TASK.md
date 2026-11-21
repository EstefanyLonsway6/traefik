# Bounty Fix

**Reward:** $10.00
**Issue:** https://github.com/EstefanyLonsway6/traefik/issues/1
**Repo:** https://github.com/EstefanyLonsway6/traefik

---

## Issue

🎯 Fix: FastProxy appends "Internal Server Error" to body when upstream closes during streaming

## 📝 Description
When utilizing the FastProxy engine for streaming responses (such as Server-Sent Events (SSE) or long-lived chunked transfers), an abrupt closure of the upstream connection causes the proxy to append "Internal Server Error" directly to the response body. 

Because the HTTP headers (typically `200 OK`) have already been flushed to the client to initiate the stream, the proxy cannot modify the HTTP status code. Instead, the error handling path attempts to write the error message to the response stream. This corrupts the stream payload and prevents client-side parsers (like EventSource) from handling the connection termination cleanly.

## 🎯 Acceptance Criteria
- [ ] If the response headers have already been written/flushed to the client, the proxy must **not** append "Internal Server Error" or any other error text to the response body.
- [ ] In the event of an upstream read error or premature closure during an active stream, the proxy should abruptly terminate the client TCP connection (or close the hijacked connection) to signal truncation to the client.
- [ ] Standard (non-streaming) requests that fail before headers are written must still correctly return a `502 Bad Gateway` or `500 Internal Server Error` status code and error payload.
- [ ] Ensure compatibility with HTTP/1.1 chunked transfer encoding and HTTP/2 stream resets.

## 🛠️ Technical Specifications & Context
- **Location:** This issue likely resides in the custom reverse proxy or buffer copy utility within the proxy middleware (e.g., `pkg/middlewares/proxy/` or a dedicated `fastproxy` package in the `EstefanyLonsway6/traefik` codebase).
- **Mechanism:** 
  - Look for the loop handling `io.Copy` or `io.CopyBuffer` between the upstream response body and the client `http.ResponseWriter`.
  - When `io.Copy` returns an error (e.g., `unexpected EOF`, `connection reset by peer`), the code currently falls back to an error handler that calls `http.Error()` or writes directly to the response writer.
  - Implement a check to determine if bytes have already been written (e.g., checking a custom tracking response writer or checking if headers are sent) before attempting to write an error response. If headers are sent, log the error and close the connection/hijacker immediately.

## 🧪 Verification & Testing
- **Integration Test:**
  1. Set up a mock upstream server that returns a `200 OK` response with `Transfer-Encoding: chunked`.
  2. Have the upstream send 2 chunks of data, pause, and then abruptly close the TCP connection without sending the final terminating chunk (`0\r\n\r\n`).
  3. Query the mock upstream through the Traefik/FastProxy instance.
  4. Verify that the client receives the first 2 chunks and that the connection is terminated by the proxy.
  5. Assert that the string "Internal Server Error" is **not** present anywhere in the received body.

---

[![Opire Bounty](https://img.shields.io/badge/bounty-Opire-emerald?style=for-the-badge)](https://github.com/EstefanyLonsway6/traefik)
<br/>
<hr/>

<details><summary>This repo is using Opire - what does it mean? 👇</summary><br/>💵 Everyone can add rewards for this issue commenting <code>/reward 100</code> (replace <code>100</code> with the amount).<br/>🕵️‍♂️ If someone starts working on this issue to earn the rewards, they can comment <code>/try</code> to let everyone know!<br/>🙌 And when they open the PR, they can comment <code>/claim #1</code> either in the PR description or in a PR's comment.<br/><br/>🪙 Also, everyone can tip any user commenting <code>/tip 20 @EstefanyLonsway6</code> (replace <code>20</code> with the amount, and <code>@EstefanyLonsway6</code> with the user to tip).<br/><br/>📖 If you want to learn more, check out our <a href="https://docs.opire.dev">documentation</a>.</details>

---

## Process

### 1. Orient yourself first
Before touching any code:
- Read the README and any relevant docs
- Run: find . -type f | grep -v node_modules | grep -v .git | grep -v dist | head -60
- Grep for the key terms from the issue title to find the relevant files
- Read the files around the bug — understand what's currently happening

### 2. Find and read the tests
- Look for test files near the relevant code (*.test.ts, *.spec.js, __tests__, etc.)
- Read them — they tell you what behavior is expected
- Understand what a passing fix looks like before writing a line

### 3. Implement the fix
- Make the MINIMAL change that fixes the issue — nothing more
- Do not refactor, rename, reorganize, or clean up unrelated code
- Do not add dependencies unless the issue explicitly requires it
- Do not modify config files (package.json, tsconfig.json, etc.) unless that IS the fix
- One logical change, one commit

### 4. Verify
- Run the test suite: check package.json scripts for "test", "lint", "typecheck"
  Common commands: npm test, yarn test, pnpm test, pytest, go test ./..., cargo test
- If TypeScript is present: npx tsc --noEmit
- Fix any test failures your change caused — if a test was wrong before, update it

### 5. Commit and document
- git add -A
- git commit -m "fix: 🎯 Fix: FastProxy appends "Internal Server Error" to body when upstream "
- Write AGENT_SUMMARY.md with:
  - Root cause: what was wrong and why
  - Fix: what you changed and the reasoning
  - Verification: how to confirm it works
  - Concerns: anything a reviewer should know

---

## Stop conditions

If ANY of these are true, write "UNCERTAIN: [reason]" at the top of AGENT_SUMMARY.md, make NO code changes, and stop:
- The fix requires changing more than ~5 files
- You cannot determine what the correct behavior should be
- The issue requires a database migration, API key, or external service
- You find 3+ open PRs already attempting this fix (high competition)
- The repo is not in a language/framework you can reason about confidently

A clean UNCERTAIN is better than a bad PR.
