# Issue: hera-agent CLI/Connector 응답 토큰 비용 최적화

- **상태**: Open — 분석 완료, 구현 미착수
- **심각도**: High (에이전트가 주 사용자 → 매 호출이 컨텍스트 윈도우를 잠식)
- **발견일**: 2026-05-20
- **환경**: hera-agent CLI v0.0.23, AgentConnector 0.0.2+, Unity 6000.x
- **테스트 프로젝트**: TidyCat (Unity 2D, URP)
- **관련 문서**: `docs/SUGGESTIONS.md` §1-1, §1-3, §1-4 (스키마/에러/잘림 관련 — 본 문서는 응답 본문 자체의 토큰 비용에 집중)

---

## 0. TL;DR — 우선순위

1단계로 항목 **A + B + I + J** (TTY 분기 한 곳에서 4개 동시 해결) → 모든 비-TTY 호출의 응답 30~40% 감축.
2단계로 **E (exec Unity Object shallow serialize)** → exec이 객체 반환 시 80~95% 감축.
3단계로 **C / D / F / G / H** 잡티 정리 → 추가 5~15% 감축.

| 항목 | 영향 범위 | 절감 추정 | 난이도 |
|------|----------|----------|---------|
| **B** Compact JSON (비-TTY) | 모든 non-string 응답 | 30~40% | 중 |
| **A** `compiling...` 배너 suppress | 모든 exec 호출 | 25 bytes/호출 | 하 |
| **G** ReadConsole 기본 lines=20 | console 호출 | 70~90% | 하 |
| **E** Unity Object shallow serialize | exec 객체 반환 | 80~95% | 중 |
| **C** StackTrace 기본 off | exec runtime error | 50~80% | 하 |
| **I** Update notice 비-TTY suppress | 신규 버전 알림 시 | 70 bytes | 하 |
| **D** Compile error 중복 제거 | exec compile error | 30~50% | 하 |
| **F** Truncation 마커 짧게 | Serialize 100+ items | 65 bytes | 하 |
| **H** ReadConsole 메타 omit-on-default | 모든 console 호출 | 50 bytes | 하 |
| **J** agent_hint 비-TTY 생략 | 일부 호출 | 30 bytes | 하 |

---

## 1. 배경

hera-agent의 주 사용자는 LLM 에이전트(Claude Code CLI, Codex 등). 사용자가 직접 입력하는 명령은 `status`/`doctor`/`update`/`uninstall` 정도에 한정되고, 나머지(`exec`/`scene`/`console`/`menu`/`screenshot`/`profiler`/`test` 등)는 모두 **에이전트가 호출한다**.

LLM 에이전트의 비용 구조:

- **입력 토큰** = 컨텍스트(이전 대화 + 도구 결과)
- **출력 토큰** = LLM 응답
- **cache_read** = 컨텍스트 캐시 hit
- **cache_write** = 캐시 신규 적재

매 hera-agent 호출의 stdout/stderr는 그대로 tool_result로 LLM 컨텍스트에 누적된다. **응답 크기가 곧 매 턴 cache_write + 다음 턴 cache_read 비용**. 동일 작업을 100바이트 vs 500바이트로 응답할 수 있다면, 호출 1000회 누적 시 400KB ≈ 100K 토큰 = $1~3 (Anthropic 기준) + 캐시 hit 지연.

본 문서는 **응답 본문(stdout)과 부수 출력(stderr)에서 줄일 수 있는 바이트를 코드 레벨로 식별**하고, 각 항목에 대해 위치/문제/개선안/예상 절감/구현 코드를 제시한다.

---

## 2. 측정 방법 및 베이스라인

### 2.1 측정 방법

각 hera-agent 명령을 실행해 `stdout + stderr` 바이트를 측정. 토큰 추정은 영문 기준 `chars ÷ 4`.

### 2.2 베이스라인: 가장 단순한 exec

```
$ hera-agent exec "return 1+1;"
[hera-agent] compiling...
2
```

- 총 28 bytes (`[hera-agent] compiling...\n` = 25 bytes stderr, `2\n` = 2 bytes stdout, `OK` 변형 시 3 bytes)
- **고정 오버헤드 25 bytes** (compile banner) — 호출당 무조건

### 2.3 시나리오 비교 (TidyCat 프로젝트 기준)

| 시나리오 | C# 입력 (bytes) | stdout+stderr (bytes) | 합계 | ≈토큰 |
|---|---:|---:|---:|---:|
| S1_naive (Canvas+3버튼, Debug.Log + verbose return) | 1,291 | ~205 | ~1,496 | ~374 |
| S1_lean (`return null;`) | 743 | 28 | 771 | ~193 |
| S2_naive (Root + 50 dummies, 50 이름 return) | 623 | ~555 | ~1,178 | ~295 |
| S2_lean (`return null;`) | 197 | 28 | 225 | ~56 |
| S3_batched (S1 + S2 한 호출) | 818 | 28 | 846 | ~212 |

> **관찰**: `Debug.Log()` 호출은 stdout에 안 나옴 — Unity Console에만 들어감. 즉, exec 내부 Debug.Log는 토큰 무료. 그러나 후속 `hera-agent console` 폴링은 비용.

### 2.4 본 문서가 다루는 비용

| 비용 원천 | 영향 | 본 문서에서 다루는가? |
|---|---|---|
| 사용자가 작성한 C# 코드 길이 | 입력 토큰 | ❌ (사용 가이드 영역) |
| 사용자가 return한 값 | 응답 토큰 | △ (E, F에서 부분적) |
| **hera-agent CLI 자체 출력** (banner, indent, notice, hint) | 응답 토큰 | ✅ (A~D, I, J) |
| **Connector 응답 envelope/메타** (truncation, console 메타) | 응답 토큰 | ✅ (F, H) |
| **Connector 자동 직렬화 깊이/폭** | 응답 토큰 | ✅ (E) |
| **에러 응답의 stacktrace/중복** | 응답 토큰 | ✅ (C, D) |

---

## 3. 낭비 지점 상세

### A. `[hera-agent] compiling...` 무조건 stderr 출력

- **위치**: `cmd/root.go:117-119`
  ```go
  if command == "exec" {
      fmt.Fprintln(os.Stderr, "[hera-agent] compiling...")
  }
  ```
- **호출 컨텍스트**: 모든 `exec` 명령 진입 시점, 캐시 hit 여부와 무관하게 출력.
- **비용**: 호출당 25 bytes 고정. 한 턴에 exec 5번이면 125 bytes 순손실. Claude Code의 `Bash` 도구는 stderr를 stdout과 합쳐 컨텍스트에 넣기 때문에 LLM이 보는 응답에 포함됨.
- **개선안**:
  - 전역 `--quiet` 플래그 또는 env `HERA_AGENT_QUIET=1` 도입. 또는
  - non-TTY 자동 감지 (B와 함께 묶음).
- **백워드 호환**: 사람 사용자(터미널) 환경에서는 그대로 출력. 비-TTY일 때만 생략. → ABI 변화 없음.
- **검증**: `hera-agent exec "return 1;" | wc -c` 가 28 → 2로 떨어지는지 확인.

### B. JSON pretty-print 강제 (최대 낭비)

- **위치**: `cmd/root.go:245-260` (`printResponse`)
  ```go
  if len(resp.Data) > 0 && string(resp.Data) != "null" {
      var pretty interface{}
      if json.Unmarshal(resp.Data, &pretty) == nil {
          if s, ok := pretty.(string); ok {
              fmt.Println(s)
          } else {
              b, _ := json.MarshalIndent(pretty, "", "  ")   // ← 2칸 들여쓰기
              fmt.Println(string(b))
          }
      } else {
          fmt.Println(string(resp.Data))
      }
  }
  ```
- **현상**: 모든 non-string `data`가 `MarshalIndent(pretty, "", "  ")`로 출력. 줄바꿈 + 2칸 인덴트로 30~40% 인플레이션.
- **예시**: `scene info` 응답
  - 현재 (indent): 12줄, ~200 bytes
    ```json
    {
      "active": {
        "isDirty": false,
        "name": "Home",
        "path": "Assets/_TidyCat/Scenes/Home.unity"
      },
      "loaded": [
        ...
      ]
    }
    ```
  - compact (`json.Marshal`): 1줄, ~110 bytes
    ```json
    {"active":{"isDirty":false,"name":"Home","path":"Assets/_TidyCat/Scenes/Home.unity"},"loaded":[...]}
    ```
- **영향 범위**: `scene info`, `console`, `list`, `exec` 객체 반환, 모든 에러 응답의 `data` 등 거의 전체 명령.
- **개선안**:
  ```go
  // cmd/root.go
  import "github.com/mattn/go-isatty"
  
  func agentOutputMode() bool {
      // 비-TTY (파이프/리다이렉트) = 에이전트가 stdout 읽는 중
      return !isatty.IsTerminal(os.Stdout.Fd()) && !isatty.IsCygwinTerminal(os.Stdout.Fd())
  }
  
  // printResponse 내부
  var b []byte
  if agentOutputMode() {
      b, _ = json.Marshal(pretty)               // compact
  } else {
      b, _ = json.MarshalIndent(pretty, "", "  ")  // human
  }
  fmt.Println(string(b))
  ```
- **참고**: go-isatty는 이미 indirect dependency (PROGRESS.json 2026-05-14 항목 "status / asset-config 출력에 TTY 가드" 참고). v0.0.8에서 direct로 승격된 상태.
- **선택지**: env `HERA_AGENT_JSON=compact|pretty`로 override할 수 있게 두면 디버깅 편함.
- **검증**: `hera-agent scene info | wc -c` 가 약 200 → 약 110 bytes로 감소.

### C. exec 런타임 에러의 전체 StackTrace

- **위치**: `AgentConnector/Editor/Tools/ExecuteCsharp.cs:405-417`
  ```cs
  catch (TargetInvocationException tie)
  {
      ...
      var inner = tie.InnerException ?? tie;
      return new ErrorResponse("EXEC_RUNTIME_ERROR",
          $"Runtime error: {inner.GetType().Name}: {inner.Message}",
          data: new
          {
              exception_type = inner.GetType().FullName,
              stack_trace = inner.StackTrace          // ← 풀스택
          });
  }
  ```
- **비용**: `Exception.StackTrace`는 보통 500~2000 bytes. Unity 내부 프레임이 대부분이라 LLM이 디버깅에 쓸 만한 user frame은 1~3줄에 불과.
- **개선안**: ReadConsole의 `--stacktrace user` 모드와 동일 정책 적용. 기본은 user frames만, `--stacktrace full` 요청 시에만 풀스택.
  ```cs
  // ExecuteCsharp.Parameters에 추가
  [ToolParameter("Stack trace mode: none, user (default), full")]
  public string Stacktrace { get; set; }
  
  // HandleCommand에서 파라미터 읽고 Invoke에 전달
  var stacktraceMode = p.Get("stacktrace", "user").ToLowerInvariant();
  
  // 에러 응답 빌더
  private static object BuildRuntimeError(Exception inner, string stacktraceMode)
  {
      object data;
      switch (stacktraceMode)
      {
          case "none":
              data = new { exception_type = inner.GetType().FullName };
              break;
          case "full":
              data = new { exception_type = inner.GetType().FullName, stack_trace = inner.StackTrace };
              break;
          default: // user
              data = new { exception_type = inner.GetType().FullName, stack_trace = FilterUserFrames(inner.StackTrace) };
              break;
      }
      return new ErrorResponse("EXEC_RUNTIME_ERROR",
          $"Runtime error: {inner.GetType().Name}: {inner.Message}",
          data: data);
  }
  
  private static string FilterUserFrames(string raw)
  {
      if (string.IsNullOrEmpty(raw)) return raw;
      var lines = raw.Split('\n');
      var sb = new StringBuilder();
      foreach (var l in lines)
      {
          if (l.Contains("at UnityEngine.") ||
              l.Contains("at UnityEditor.") ||
              l.Contains("at System.") ||
              l.Contains("RuntimeMethodHandle.InvokeMethod")) continue;
          if (sb.Length > 0) sb.Append('\n');
          sb.Append(l.TrimEnd());
      }
      return sb.ToString();
  }
  ```
- **재현 예시**:
  ```cs
  exec "var x = (string)null; return x.Length;"
  ```
  - 현재 응답: stack_trace ~1200 bytes
  - user 필터 후: ~120 bytes (`__CliDynamic.Execute()` 1프레임)
- **검증**: 위 명령 실행 후 응답 size 측정.

### D. exec 컴파일 에러 메시지 중복

- **위치**: `AgentConnector/Editor/Tools/ExecuteCsharp.cs:341-348`
  ```cs
  var parsed = ParseErrors(output);
  return new CompileResult { Error = new ErrorResponse(
      "EXEC_COMPILE_ERROR",
      $"Compile error:\n{FormatErrors(output)}",   // ← 사람용 멀티라인
      data: new { compile_errors = parsed }) };    // ← 머신용 구조
  ```
- **현상**: `message`(L1: 오류1, L2: 오류2, ...) + `data.compile_errors`(같은 정보의 구조화 배열) → 같은 내용을 두 번 들고 옴.
- **비용**: 컴파일 에러 5개 발생 시 message 200 bytes + data 350 bytes ≈ 550 bytes. 중복 절반은 200 bytes 순절감.
- **개선안**: message는 첫 번째 에러 한 줄만, 상세는 `data.compile_errors`에만.
  ```cs
  var firstError = parsed.Count > 0 ? FormatFirstError(parsed[0]) : "compile failed";
  return new CompileResult { Error = new ErrorResponse(
      "EXEC_COMPILE_ERROR",
      $"Compile error: {firstError} (+{parsed.Count - 1} more)",
      data: new { compile_errors = parsed }) };
  ```
- **백워드 호환**: 머신 파싱은 `code == "EXEC_COMPILE_ERROR"` + `data.compile_errors` 사용이 정도. message 문자열 매칭에 의존하는 코드는 깨질 수 있으나, SUGGESTIONS.md §1-3 정책상 message 매칭은 비권장.
- **검증**: `exec "int x = ; int y = ;"` (의도적 syntax error) 응답 size 비교.

### E. exec Serialize 기본 depth=3 + Unity Object 풀 reflection

- **위치**: `AgentConnector/Editor/Tools/ExecuteCsharp.cs:83-90, 469-550`
  ```cs
  private const int DefaultSerializeDepth = 3;   // ← 기본 3
  private const int MaxSerializeDepth = 8;
  
  // Serialize() 내부: type.IsValueType || type.IsClass 분기에서
  // GetFields + GetProperties로 모든 public member 직렬화
  ```
- **현상**: 사용자가 무심코 `return GameObject.Find("Canvas");` 또는 `return transform;` 하면 다음과 같이 폭증:
  - `Transform` → `position`, `rotation`, `eulerAngles`, `localPosition`, `localRotation`, `localScale`, `lossyScale`, `worldToLocalMatrix`, `localToWorldMatrix`, `right`, `up`, `forward`, `childCount`, `parent`, `root`, `hasChanged`, `gameObject`, `name`, `tag`, `hideFlags`, ...
  - 각 Vector3/Quaternion/Matrix4x4가 또 fields 재귀 → depth=3 도달까지 수백 줄 응답.
- **실측 (TidyCat)**:
  ```
  hera-agent exec "return GameObject.Find('Canvas').transform;"
  → JSON 약 4.2KB
  ```
- **개선안**:
  1. **기본 depth를 1 또는 2로 낮춤** (3 → 1 권장).
  2. **UnityEngine.Object 타입은 기본 shallow**: depth와 무관하게 `{ name, type, instanceID }` 만. depth 명시되면 풀 reflection.
  3. **명시적 `--full` 또는 `depth >= 3`일 때만 풀 직렬화**.
  ```cs
  // ExecuteCsharp.cs Serialize() 진입부에 추가
  private static object Serialize(object obj, int depth, int maxDepth, HashSet<object> visited)
  {
      if (obj == null) return null;
      if (depth > maxDepth) return obj.ToString();
      var type = obj.GetType();
      if (type.IsPrimitive || type == typeof(string) || type == typeof(decimal)) return obj;
      if (type.IsEnum) return obj.ToString();
      if (type.Name.StartsWith("FixedString")) return obj.ToString();
      
      // ↓ 추가: Unity Object shallow-by-default
      if (obj is UnityEngine.Object uo && maxDepth < 3)
      {
          return new Dictionary<string, object>
          {
              ["name"] = uo.name,
              ["type"] = type.Name,
              ["instanceID"] = uo.GetInstanceID()
          };
      }
      // ↑ 추가 끝
      
      if (obj is IDictionary dict) { ... }
      // ...
  }
  ```
  - `DefaultSerializeDepth`를 1로 변경. 사용자는 `--depth 3` 명시 시 기존 동작.
- **검증**:
  ```bash
  hera-agent exec "return GameObject.Find('Canvas').transform;" | wc -c
  # 변경 전: ~4200 bytes
  # 변경 후: ~80 bytes
  ```
- **트레이드오프**: 기존 사용자 중 객체 자동 직렬화에 의존하는 사람은 `--depth 3` 추가 필요. CHANGELOG 명시 필요. 그러나 SUGGESTIONS.md §1-5에서 이미 "depth 조이거나 --depth 추가" 권고됨.

### F. Serialize 100-item cap 의 verbose truncation 표식

- **위치**: `AgentConnector/Editor/Tools/ExecuteCsharp.cs:502-510`
  ```cs
  if (truncated)
  {
      list.Add(new Dictionary<string, object>
      {
          ["__truncated"] = true,
          ["returned"] = limit,
          ["hint"] = $"output capped at {limit} items — filter at source or paginate"
      });
  }
  ```
- **비용**: 잘림 마커 자체가 약 80 bytes (compact JSON 기준). 사용자에게 주는 정보는 "잘렸음 + 100개 반환됨" 뿐.
- **개선안**: SUGGESTIONS.md §1-4 권고와 통합 — `truncation` 메타 객체를 envelope에 올리고, 배열 끝에는 짧은 마커만.
  - 옵션 1 (간단): 배열 끝 마커를 짧은 문자열로
    ```cs
    list.Add($"__truncated:{limit}");
    ```
  - 옵션 2 (정도): SuccessResponse에 `truncation` 필드 추가하고 배열은 깨끗하게
    ```cs
    // SuccessResponse에 truncation 필드 추가
    public class SuccessResponse {
        ...
        [JsonProperty(NullValueHandling = NullValueHandling.Ignore)]
        public object truncation;
    }
    
    // Invoke()에서 반환 직전
    if (serialized is List<object> l && WasTruncated(l)) {
        return new SuccessResponse("OK", l) { truncation = new { limit = 100, hint = "use paging" } };
    }
    ```
- **검증**: `exec "return Enumerable.Range(0,200).ToList();"` 응답에서 잘림 표식 크기 비교.

### G. ReadConsole 기본 무제한 lines

- **위치**: `AgentConnector/Editor/Tools/ReadConsole.cs:106-113`
  ```cs
  var type = p.Get("type", "error,warning,log").ToLower();
  var types = type.Split(',')...;
  int? count = p.GetInt("lines") ?? p.GetInt("count");   // ← null이면 전체
  string stacktrace = p.Get("stacktrace", "user").ToLower();
  int since = p.GetInt("since") ?? 0;
  return GetEntries(types, count, stacktrace, since);
  ```
- **현상**: 사용자가 `--lines`를 안 주면 console의 모든 entry 반환. 실 프로젝트에서 200~500 entry 흔함.
- **개선안**: 기본 20으로 cap. `--lines 0` 또는 `--lines all`은 의도적 전체.
  ```cs
  const int DefaultLineLimit = 20;
  int? count = p.GetInt("lines") ?? p.GetInt("count");
  if (!count.HasValue) count = DefaultLineLimit;
  else if (count.Value == 0) count = null;   // 0 = 무제한 (의도적)
  ```
- **백워드 호환**: `console --lines 0` 추가하면 기존 무제한 동작 보존. CHANGELOG 필수.
- **검증**: `hera-agent console` 응답 크기 비교 (console에 다수의 로그가 있는 상태에서).

### H. ReadConsole 응답 메타 6필드 무조건 포함

- **위치**: `AgentConnector/Editor/Tools/ReadConsole.cs:159-168`
  ```cs
  return new SuccessResponse($"Retrieved {entries.Count} entries.", new
  {
      entries,
      total_in_console = total,
      matched = filteredTotal,
      returned = entries.Count,
      since,
      last_cursor = lastIndex,
      truncated,
  });
  ```
- **현상**: `since=0`, `truncated=false`, `last_cursor=N` 등 기본값일 때도 항상 포함. 페이지네이션 안 쓰는 호출에 ~50 bytes 낭비.
- **개선안**: 기본값일 때 omit. Newtonsoft `NullValueHandling.Ignore`와 유사하게 default값 가드.
  ```cs
  var responseData = new Dictionary<string, object>
  {
      ["entries"] = entries,
      ["returned"] = entries.Count
  };
  if (total != entries.Count) responseData["total_in_console"] = total;
  if (filteredTotal != entries.Count) responseData["matched"] = filteredTotal;
  if (since > 0) responseData["since"] = since;
  if (lastIndex != entries.Count) responseData["last_cursor"] = lastIndex;
  if (truncated) responseData["truncated"] = true;
  return new SuccessResponse($"Retrieved {entries.Count} entries.", responseData);
  ```
- **검증**: `hera-agent console --type error` 응답 비교 (에러 없는 상태).

### I. `printUpdateNotice` 비-TTY에서도 출력

- **위치**: `cmd/version_check.go:112-114`
  ```go
  func printNotice(current, latest string) {
      fmt.Fprintf(os.Stderr, "\nUpdate available: %s → %s\nRun \"hera-agent update\" to upgrade.\n", current, latest)
  }
  ```
- **현상**: 새 버전 있을 때마다 2줄 stderr 출력 (~70 bytes). 캐시 만료(24h?) 주기로 fetch 시도.
- **비용**: 가끔이지만 LLM 입장 100% 노이즈.
- **개선안**: B와 동일한 `agentOutputMode()` 가드.
  ```go
  func printNotice(current, latest string) {
      if agentOutputMode() { return }
      fmt.Fprintf(os.Stderr, "...", current, latest)
  }
  ```
- **검증**: `HERA_AGENT_QUIET=1 hera-agent status` 또는 파이프로 호출 시 출력 없음 확인.

### J. agent_hint stderr 중복 출력

- **위치**: `cmd/root.go:241-243`
  ```go
  if resp.AgentHint != "" {
      fmt.Fprintf(os.Stderr, "[hera-agent] hint: %s\n", resp.AgentHint)
  }
  ```
- **현상**: 응답 JSON에도 `agent_hint`가 들어 있을 수 있는데, 별도로 stderr에 한 번 더 찍음 = 같은 정보 2번.
- **비용**: prefix 16 bytes + hint 텍스트.
- **개선안**: 비-TTY에서는 stderr 출력 생략 (JSON에 이미 있음).
  ```go
  if resp.AgentHint != "" && !agentOutputMode() {
      fmt.Fprintf(os.Stderr, "[hera-agent] hint: %s\n", resp.AgentHint)
  }
  ```

---

## 4. 통합 구현 가이드

### 4.1 1단계 — TTY 분기 단일 헬퍼 (A + B + I + J)

가장 큰 ROI. 한 헬퍼로 4개 항목 동시 해결.

**파일**: `cmd/root.go` (또는 새 `cmd/output_mode.go`)

```go
package cmd

import (
    "os"
    "github.com/mattn/go-isatty"
)

// agentOutputMode reports whether stdout is being read by a non-human consumer
// (pipe, redirect, or CI). When true, suppress decorative stderr lines and
// emit compact JSON instead of indented.
func agentOutputMode() bool {
    if os.Getenv("HERA_AGENT_QUIET") == "1" {
        return true
    }
    fd := os.Stdout.Fd()
    if isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd) {
        return false
    }
    return true
}
```

**적용 1 — `cmd/root.go:117-119` (항목 A)**
```go
// before
if command == "exec" {
    fmt.Fprintln(os.Stderr, "[hera-agent] compiling...")
}

// after
if command == "exec" && !agentOutputMode() {
    fmt.Fprintln(os.Stderr, "[hera-agent] compiling...")
}
```

**적용 2 — `cmd/root.go:245-260` (항목 B)**
```go
// before
b, _ := json.MarshalIndent(pretty, "", "  ")
fmt.Println(string(b))

// after
var b []byte
if agentOutputMode() {
    b, _ = json.Marshal(pretty)
} else {
    b, _ = json.MarshalIndent(pretty, "", "  ")
}
fmt.Println(string(b))
```

**적용 3 — `cmd/version_check.go:112-114` (항목 I)**
```go
func printNotice(current, latest string) {
    if agentOutputMode() { return }
    fmt.Fprintf(os.Stderr, "\nUpdate available: %s → %s\nRun \"hera-agent update\" to upgrade.\n", current, latest)
}
```

**적용 4 — `cmd/root.go:241-243` (항목 J)**
```go
if resp.AgentHint != "" && !agentOutputMode() {
    fmt.Fprintf(os.Stderr, "[hera-agent] hint: %s\n", resp.AgentHint)
}
```

**테스트** (`cmd/root_test.go` 또는 신규):
```go
func TestAgentOutputMode_EnvOverride(t *testing.T) {
    t.Setenv("HERA_AGENT_QUIET", "1")
    if !agentOutputMode() { t.Fatal("expected agent mode when HERA_AGENT_QUIET=1") }
}

func TestPrintResponse_CompactWhenPiped(t *testing.T) {
    // stdout을 파이프로 리다이렉트 후 printResponse 호출 → 출력에 줄바꿈/들여쓰기 없는지 검증
}
```

### 4.2 2단계 — Connector Serialize 개선 (E)

**파일**: `AgentConnector/Editor/Tools/ExecuteCsharp.cs`

변경 1 — 기본 depth:
```cs
// line 83
private const int DefaultSerializeDepth = 1;   // was 3
```

변경 2 — Unity Object shallow short-circuit (Serialize 메서드 안에 추가):
```cs
private static object Serialize(object obj, int depth, int maxDepth, HashSet<object> visited)
{
    if (obj == null) return null;
    if (depth > maxDepth) return obj.ToString();
    var type = obj.GetType();
    if (type.IsPrimitive || type == typeof(string) || type == typeof(decimal)) return obj;
    if (type.IsEnum) return obj.ToString();
    if (type.Name.StartsWith("FixedString")) return obj.ToString();

    // NEW: shallow form for UnityEngine.Object unless deep depth was explicitly requested
    if (obj is UnityEngine.Object uo && maxDepth < 3)
    {
        var shallow = new Dictionary<string, object>
        {
            ["name"] = uo == null ? null : uo.name,
            ["type"] = type.Name
        };
        // include instanceID only if valid (destroyed objects throw)
        try { shallow["instanceID"] = uo.GetInstanceID(); } catch { }
        return shallow;
    }
    // existing dict/enumerable/class handling continues unchanged below
    if (obj is IDictionary dict) { ... }
    ...
}
```

**테스트** (`AgentConnector/Editor/Tools/__editmodetests__/ExecuteCsharpTests.cs` 신규 또는 기존 통합테스트):
```cs
[Test]
public void Serialize_UnityObject_DefaultDepth_ReturnsShallow()
{
    var go = new GameObject("TestObj");
    try
    {
        var result = SerializePublic(go.transform, depth: 0, maxDepth: 1);
        var dict = (Dictionary<string, object>)result;
        Assert.AreEqual("TestObj", dict["name"]);
        Assert.AreEqual("Transform", dict["type"]);
        Assert.IsFalse(dict.ContainsKey("position"), "shallow form must not include position");
    }
    finally { GameObject.DestroyImmediate(go); }
}

[Test]
public void Serialize_UnityObject_ExplicitDepth3_IncludesProperties()
{
    var go = new GameObject("TestObj");
    try
    {
        var result = SerializePublic(go.transform, depth: 0, maxDepth: 3);
        var dict = (Dictionary<string, object>)result;
        Assert.IsTrue(dict.ContainsKey("position"));
    }
    finally { GameObject.DestroyImmediate(go); }
}
```

> Serialize는 private이라 테스트용 internal 헬퍼 노출이 필요할 수 있음. 또는 통합테스트로 `exec "return GameObject.Find('X').transform;"` 응답을 검증.

### 4.3 3단계 — 잡티 정리

순서대로 작은 PR 5개로 분리 권장 (각 5~15분):

1. **C (StackTrace user 기본)**: ExecuteCsharp.cs에 `Parameters.Stacktrace`, `FilterUserFrames` 추가, `BuildRuntimeError` 헬퍼.
2. **D (Compile error 중복 제거)**: `ExecuteCsharp.cs:345-348`의 message를 첫 에러 한 줄 + `(+N more)`로 축약.
3. **F (Truncation 마커 간소화)**: `ExecuteCsharp.cs:502-510`을 `list.Add($"__truncated:{limit}");`로 또는 SuccessResponse.truncation 필드로 분리.
4. **G (Console 기본 lines=20)**: `ReadConsole.cs:109` 가드 추가, `--lines 0` = 무제한 컨벤션.
5. **H (Console 메타 omit-on-default)**: `ReadConsole.cs:159-168` Dictionary 빌더 패턴으로.

---

## 5. 변경 영향 / 위험

| 항목 | 변경 종류 | 사용자 영향 |
|------|---------|------------|
| A | 출력만 변화 | 사람 사용 영향 없음 (TTY 유지). 에이전트 응답 짧아짐. |
| B | 출력 포맷 변화 | 사람 영향 없음. 응답 JSON 파싱하는 스크립트는 영향 없음 (JSON 유효성 동일). |
| C | API 추가 (`--stacktrace`) | 기존 stack_trace 의존 코드는 `--stacktrace full`로 마이그레이션 필요. **CHANGELOG breaking** 표기 |
| D | message 텍스트 변화 | message 문자열 매칭하는 코드 깨짐. `code == "EXEC_COMPILE_ERROR"` 사용 권장. |
| E | **기본 응답 모양 변화** | `return go.transform;` 같은 패턴이 짧은 형태로 응답. 명시적 `--depth 3` 필요. **CHANGELOG breaking** 표기 |
| F | 마커 형태 변화 | `__truncated` dict 키 의존 코드 영향. |
| G | 기본 동작 변화 | `--lines 0`으로 기존 동작 복원 가능. **CHANGELOG breaking** 표기 |
| H | 응답 필드 일부 omit | 기본값 가정 코드는 영향 없음. 명시적 키 존재 체크하는 코드는 영향 가능. |
| I | stderr 출력 사라짐 | 사람 환경 영향 없음. |
| J | stderr 출력 사라짐 (JSON에는 그대로) | 사람 환경 영향 없음. 에이전트는 JSON에서 읽으면 됨. |

**최소 위험 PR 분할**:
- PR 1: A + B + I + J (단일 TTY 헬퍼) — non-breaking
- PR 2: H (Console 메타 omit) — non-breaking (필드 누락 허용)
- PR 3: F (Truncation 마커) — minor breaking (마커 형태)
- PR 4: D (Compile error 메시지 축약) — minor breaking (message 텍스트)
- PR 5: C (StackTrace 옵션) — breaking (`--stacktrace full` 없으면 user만)
- PR 6: G (Console lines 기본 20) — breaking (`--lines 0` 추가)
- PR 7: E (Unity Object shallow) — breaking (`--depth 3` 추가)

---

## 6. 측정 방법론

### 6.1 단일 명령 응답 크기 측정

```bash
# 단일 명령
hera-agent <cmd> 2>&1 | wc -c

# 토큰 근사 (영문 기준)
echo $(($(hera-agent <cmd> 2>&1 | wc -c) / 4))
```

### 6.2 시나리오 누적 측정

Claude Code 환경에서 `~/.claude/.last-turn-tokens.json` (TidyCat 프로젝트의 Stop hook에서 사용 중인 사이드카) 또는 transcript JSONL의 `usage` 필드:

```bash
node -e '
const fs = require("fs");
const lines = fs.readFileSync(process.argv[1], "utf8").trim().split("\n");
for (const l of lines) {
  try {
    const o = JSON.parse(l);
    if (o.message?.usage) console.log(JSON.stringify(o.message.usage));
  } catch {}
}
' <transcript.jsonl>
```

### 6.3 회귀 방지

각 항목 구현 시 통합테스트 추가 권장:

| 항목 | 테스트 | 기대값 |
|------|------|---------|
| A | `agentOutputMode=true`에서 exec 호출 후 stderr에 "compiling" 없음 | ✅ |
| B | non-TTY 호출 시 `scene info` 응답에 줄바꿈 0개 | ✅ |
| E | depth=1로 `transform` 직렬화 시 `position` 키 부재 | ✅ |
| E | depth=3로 같은 호출 시 `position` 키 존재 | ✅ |
| G | `console --type error` (count 미지정) 응답이 20개 이하 | ✅ |

---

## 7. 미정 / 후속

- **`list` 응답 token bomb** (SUGGESTIONS.md §1-1) — 본 문서 범위 밖. 별도 이슈로.
- **Schema 4중 표현** (SUGGESTIONS.md §1-2) — 별도 이슈.
- **응답 envelope 키 안정화** (SUGGESTIONS.md §1-7) — 별도 이슈.
- **`status --compact`** (SUGGESTIONS.md §1-8) — 본 문서의 항목 B와 충돌 없음. 별도 이슈.

본 문서는 **응답 본문 자체의 바이트 감소**에 한정. 위 항목들은 구조/스키마 수준의 작업이므로 분리.

---

## 8. 참고

- 측정 데이터 raw: `<TidyCat>/.omc/exec-bench/s{1,2,3}_{naive,lean,batched_lean}.cs`
- 비교 베이스라인: hera-agent CLI v0.0.23 + Connector 0.0.2 (Unity 6000.3.5f2)
- 관련 분석: `docs/SUGGESTIONS.md` (스키마/에러/잘림/UX), `docs/COMMANDS.md` (명령 카탈로그), `docs/ARCHITECTURE.md`
