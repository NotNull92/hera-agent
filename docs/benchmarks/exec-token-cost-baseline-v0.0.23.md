# Benchmark: `exec` 명령 토큰 비용 베이스라인 (v0.0.23)

- **목적**: `docs/issues/token-cost-reduction.md`의 개선안 적용 전후 비교를 위한 측정 베이스라인.
- **측정일**: 2026-05-20
- **측정 환경**:
  - hera-agent CLI **v0.0.23** (Windows / `C:\Users\PC\AppData\Local\Microsoft\WindowsApps\hera-agent.exe`)
  - AgentConnector **0.0.2+** (PROGRESS.json 기준 — 설치 버전은 v0.0.23 빌드 시점 동기화)
  - Unity **6000.3.5f2**
  - 테스트 프로젝트: TidyCat (`C:\Users\PC\Desktop\Cowork\TidyCat`)
  - 활성 씬: `Assets/_TidyCat/Scenes/Home.unity` (rootCount=7, isDirty=false)
  - Shell: PowerShell 7 / Git Bash 4.4 (Bash 도구 경유)
- **참고 문서**: 개선 제안서는 [`docs/issues/token-cost-reduction.md`](../issues/token-cost-reduction.md).

---

## 0. 요약 (Baseline Numbers)

| 시나리오 | C# 입력 (bytes) | CLI stdout+stderr (bytes) | 합계 (bytes) | ≈토큰 |
|---|---:|---:|---:|---:|
| `exec "return 1+1;"` (최소 베이스라인) | 12 | 28 | 40 | ~10 |
| `exec "return null;"` (최소 void) | 14 | 29 | 43 | ~11 |
| **S1_naive** (Canvas + UIButtonContainer + 3 UIButton, Debug.Log + verbose return) | 1,291 | ~205 | ~1,496 | **~374** |
| **S1_lean** (동일 작업, `return null;`) | 743 | 28 | 771 | **~193** |
| **S2_naive** (Root + 50 Dummy, Debug.Log + 50개 이름 return) | 623 | ~555 | ~1,178 | **~295** |
| **S2_lean** (동일 작업, `return null;`) | 197 | 28 | 225 | **~56** |
| **S3_batched** (S1+S2를 한 exec 호출에 묶음, lean) | 818 | 28 | 846 | **~212** |
| (참고) S1_lean + S2_lean 분리 호출 합 | 940 | 56 | 996 | ~249 |

> 토큰 추정은 영문 기준 `chars ÷ 4`. tool_use/tool_result 프레임 오버헤드(LLM 컨텍스트에서 추가됨)는 포함 안 됨.

**관찰 요약**:
- `[hera-agent] compiling...` 고정 25 bytes/호출 + `OK\n` 또는 결과값 → exec 1회 최소 28 bytes.
- Debug.Log은 stdout/stderr에 안 나옴 — Unity Console로만 감 → 직접 토큰 비용 0.
- 같은 작업이라도 verbose return 문자열 vs `return null;` 차이가 응답 100배 (28 vs 555 bytes).
- 두 작업을 한 exec에 묶으면 `compiling...` 배너 + Bash tool framing 1회 절감 (846 < 996).

---

## 1. 측정 방법론

### 1.1 측정 단위

- **입력 (input bytes)**: `--file <path>`로 전달한 `.cs` 파일의 raw 바이트 (UTF-8 기준 `wc -c`).
- **응답 (output bytes)**: `hera-agent exec --file ...` 호출의 stdout + stderr 결합 바이트.
  - Claude Code의 Bash 도구는 stdout/stderr를 합쳐 컨텍스트에 넣기 때문에 합산 측정이 적절.
- **토큰 추정**: 영문/숫자 기준 `chars ÷ 4` (OpenAI/Anthropic tokenizer 평균치).

### 1.2 측정 절차 (재현)

```bash
# 1. 측정 디렉토리 준비
mkdir -p .omc/exec-bench

# 2. 시나리오 C# 파일 작성 (§3 참고)

# 3. 각 시나리오별 입력 바이트 측정
wc -c < .omc/exec-bench/s1_ui_naive.cs

# 4. 응답 바이트 측정 (stdout+stderr 합산)
hera-agent exec --file .omc/exec-bench/s1_ui_naive.cs 2>&1 | wc -c

# 5. 토큰 근사
echo $(($(hera-agent exec --file .omc/exec-bench/s1_ui_naive.cs 2>&1 | wc -c) / 4))
```

### 1.3 사전 조건

- hera-agent와 Unity가 ready 상태:
  ```bash
  hera-agent status
  # → Unity (port 8090): ready
  ```
- 첫 컴파일은 csc 콜드 스타트 비용이 있으므로 **두 번째 실행값**을 측정값으로 사용.
- 측정 사이 씬 상태가 누적되지 않도록 클린업(§5)을 권장.

### 1.4 측정 안 한 비용

본 벤치에서 **포함하지 않은** 토큰 비용 (LLM 컨텍스트에선 발생):
- `tool_use` 호출 프레임 (Bash tool 호출 자체의 ~50~150 토큰)
- `tool_result` 응답 프레임 (~30~80 토큰)
- 사용자 프롬프트, 시스템 프롬프트
- 동일 컨텍스트 재사용 시 cache_read 할인

이들은 CLI 개선과 무관하므로 본 벤치 범위 밖. CLI/Connector 응답 크기 자체만 측정한다.

---

## 2. 베이스라인 실측 데이터

### 2.1 최소 호출 (warm-up baseline)

| 명령 | stdout | stderr | 합계 |
|---|---|---|---:|
| `hera-agent exec "return 1+1;"` | `2\n` (2 bytes) | `[hera-agent] compiling...\n` (25 bytes) | **28** bytes |
| `hera-agent exec "return null;"` | `OK\n` (3 bytes) | `[hera-agent] compiling...\n` (25 bytes) | **29** bytes |

**해석**: exec 호출당 **25 bytes 고정 stderr 오버헤드**가 있고, 응답 본문은 짧으면 2~3 bytes.

### 2.2 시나리오 S1 — UI 계층 생성

**의도**: "빈 캔버스 오브젝트 만들고 UIButtonContainer 오브젝트 만든 다음에 하위에 UIButton 오브젝트 3개 만들어줘"

#### S1_naive: 매 작업마다 Debug.Log + verbose return

- **입력**: 1,291 bytes
- **stdout**: `Canvas created with UIButtonContainer containing buttons: UIButton_0, UIButton_1, UIButton_2. Hierarchy: Canvas > UIButtonContainer > [UIButton_0, UIButton_1, UIButton_2]\n` (~180 bytes)
- **stderr**: `[hera-agent] compiling...\n` (25 bytes)
- **합계**: ~1,496 bytes (~374 토큰)
- **C# 파일**: §3.1

#### S1_lean: 동일 작업, `return null;`

- **입력**: 743 bytes
- **stdout**: `OK\n` (3 bytes)
- **stderr**: `[hera-agent] compiling...\n` (25 bytes)
- **합계**: 771 bytes (~193 토큰)
- **C# 파일**: §3.2

**S1 절감**: naive→lean = **725 bytes 절감 (48% 감소, ~181 토큰)**.

### 2.3 시나리오 S2 — Root + 50 Dummy

**의도**: "Root 라는 게임 오브젝트 만들고 Dummy 오브젝트 50개 만들어줘"

#### S2_naive: 매 iteration Debug.Log + 50개 이름 return

- **입력**: 623 bytes
- **stdout**: `Created Root with 50 children: Dummy_0, Dummy_1, ..., Dummy_49\n` (~530 bytes)
- **stderr**: 25 bytes
- **합계**: ~1,178 bytes (~295 토큰)
- **C# 파일**: §3.3

#### S2_lean: 동일 작업, `return null;`

- **입력**: 197 bytes
- **stdout**: `OK\n` (3 bytes)
- **stderr**: 25 bytes
- **합계**: 225 bytes (~56 토큰)
- **C# 파일**: §3.4

**S2 절감**: naive→lean = **953 bytes 절감 (81% 감소, ~239 토큰)**.

### 2.4 시나리오 S3 — 두 작업 배치

#### S3_batched_lean: S1 + S2 을 한 exec 호출에

- **입력**: 818 bytes
- **stdout**: `OK\n` (3 bytes)
- **stderr**: 25 bytes
- **합계**: 846 bytes (~212 토큰)
- **C# 파일**: §3.5

**S3 vs (S1_lean + S2_lean) 분리 호출**:
- 배치: 846 bytes (1 호출)
- 분리: 771 + 225 = 996 bytes (2 호출)
- **150 bytes 절감 (~37 토큰)** — `compiling...` 1회 + 결과 라인 1회 절감. tool_use/tool_result framing까지 합치면 실제 LLM 컨텍스트 절감은 더 크다.

---

## 3. 시나리오 C# 코드 (재현용)

### 3.1 `s1_ui_naive.cs`

```cs
// Scenario 1A: Canvas + UIButtonContainer + 3 UIButton (NAIVE - verbose)
Debug.Log("Starting UI creation...");

var canvasGO = new GameObject("Canvas");
var canvas = canvasGO.AddComponent<Canvas>();
canvas.renderMode = RenderMode.ScreenSpaceOverlay;
canvasGO.AddComponent<UnityEngine.UI.CanvasScaler>();
canvasGO.AddComponent<UnityEngine.UI.GraphicRaycaster>();
Debug.Log("Created Canvas: " + canvasGO.name);

var container = new GameObject("UIButtonContainer", typeof(RectTransform));
container.transform.SetParent(canvasGO.transform, false);
Debug.Log("Created UIButtonContainer under " + canvasGO.name);

var created = new List<string>();
for (int i = 0; i < 3; i++) {
    var btnGO = new GameObject("UIButton_" + i, typeof(RectTransform));
    btnGO.AddComponent<UnityEngine.UI.Image>();
    btnGO.AddComponent<UnityEngine.UI.Button>();
    btnGO.transform.SetParent(container.transform, false);
    Debug.Log("Created button: " + btnGO.name + " under " + container.name);
    created.Add(btnGO.name);
}

Debug.Log("UI creation complete. Total buttons: " + created.Count);
return "Canvas created with " + container.name + " containing buttons: " + string.Join(", ", created) +
       ". Hierarchy: " + canvasGO.name + " > " + container.name + " > [" + string.Join(", ", created) + "]";
```

### 3.2 `s1_ui_lean.cs`

```cs
// Scenario 1B: Canvas + UIButtonContainer + 3 UIButton (LEAN - silent, summary only)
var canvasGO = new GameObject("Canvas");
var canvas = canvasGO.AddComponent<Canvas>();
canvas.renderMode = RenderMode.ScreenSpaceOverlay;
canvasGO.AddComponent<UnityEngine.UI.CanvasScaler>();
canvasGO.AddComponent<UnityEngine.UI.GraphicRaycaster>();
var container = new GameObject("UIButtonContainer", typeof(RectTransform));
container.transform.SetParent(canvasGO.transform, false);
for (int i = 0; i < 3; i++) {
    var btnGO = new GameObject("UIButton_" + i, typeof(RectTransform));
    btnGO.AddComponent<UnityEngine.UI.Image>();
    btnGO.AddComponent<UnityEngine.UI.Button>();
    btnGO.transform.SetParent(container.transform, false);
}
return null;
```

### 3.3 `s2_dummy_naive.cs`

```cs
// Scenario 2A: Root + 50 Dummy (NAIVE - per-object log + verbose return)
Debug.Log("Creating Root GameObject...");
var root = new GameObject("Root");
Debug.Log("Root created at " + root.transform.position);

var names = new List<string>();
for (int i = 0; i < 50; i++) {
    var dummy = new GameObject("Dummy_" + i);
    dummy.transform.SetParent(root.transform, false);
    Debug.Log("Created Dummy_" + i + " under Root (index " + i + "/50)");
    names.Add(dummy.name);
}

Debug.Log("Dummy creation complete. Total: " + names.Count);
return "Created Root with " + names.Count + " children: " + string.Join(", ", names);
```

### 3.4 `s2_dummy_lean.cs`

```cs
// Scenario 2B: Root + 50 Dummy (LEAN)
var root = new GameObject("Root");
for (int i = 0; i < 50; i++) {
    new GameObject("Dummy_" + i).transform.SetParent(root.transform, false);
}
return null;
```

### 3.5 `s3_batched_lean.cs`

```cs
// Scenario 3: S1 + S2 combined in one exec call
var canvas = new GameObject("Canvas2");
var c = canvas.AddComponent<Canvas>();
c.renderMode = RenderMode.ScreenSpaceOverlay;
canvas.AddComponent<UnityEngine.UI.CanvasScaler>();
canvas.AddComponent<UnityEngine.UI.GraphicRaycaster>();
var container = new GameObject("UIButtonContainer2", typeof(RectTransform));
container.transform.SetParent(canvas.transform, false);
for (int i = 0; i < 3; i++) {
    var b = new GameObject("UIButton_" + i, typeof(RectTransform));
    b.AddComponent<UnityEngine.UI.Image>();
    b.AddComponent<UnityEngine.UI.Button>();
    b.transform.SetParent(container.transform, false);
}
var root = new GameObject("Root2");
for (int i = 0; i < 50; i++) {
    new GameObject("Dummy_" + i).transform.SetParent(root.transform, false);
}
return null;
```

---

## 4. 개선 후 재측정 결과 (BEFORE / AFTER)

**측정 환경**:
- CLI: `hera-agent-dev.exe` (로컬 빌드, 브랜치 main + 본 PR 변경 사항)
- Connector: TidyCat manifest를 `file:../../hera-agent/AgentConnector`로 임시 전환하여 로컬 소스를 로드 (package.json version `0.0.20`, source `Local` 확인)
- 동일 Unity 세션 / 동일 활성 씬 (`Home.unity`, rootCount=15)
- 모든 명령은 `HERA_AGENT_NO_PATH_CHECK=1`로 path warning을 silenced한 상태 (베이스라인 측정 시 설치 바이너리에서는 발생하지 않던 노이즈를 제거하기 위함)
- AFTER는 기본 동작 (비-TTY 자동 감지 → agent mode), 별도 명시한 경우만 `HERA_AGENT_QUIET=0`로 human mode 강제

| 시나리오 | BEFORE (v0.0.23) | AFTER | 절감 (bytes) | 절감 (%) |
|---|---:|---:|---:|---:|
| `exec "return 1+1;"` | 28 | **2** | 26 | **93%** |
| `exec "return null;"` | 29 | **3** | 26 | **90%** |
| S1_naive (응답 부분만, stderr 25B + stdout 텍스트) | ~205 | **171** | 34 | **17%** |
| S1_lean | 28 | **3** | 25 | **89%** |
| S2_naive | ~555 | **520** | 35 | **6%** |
| S2_lean | 28 | **3** | 25 | **89%** |
| S3_batched | 28 | **3** | 25 | **89%** |

**관찰**:
- 모든 exec 호출에서 `[hera-agent] compiling...` 25 bytes가 사라짐 (항목 A).
- naive 시나리오의 절감률이 낮은 이유는 응답의 95% 이상이 **사용자 코드가 `return` 한 문자열**이라 CLI/Connector 차원에서 줄일 수 없기 때문. naive→lean 패턴 가이드가 코드 개선보다 큰 ROI (S1 48%, S2 81%).
- lean 시나리오는 응답이 거의 ‘banner+OK’만 남으므로 banner 제거가 곧 89% 절감.

### 4.1 개선안 항목별 검증 결과

| 항목 | 명령 | BEFORE | AFTER | 절감 (%) | 비고 |
|---|---|---:|---:|---:|---|
| **A** Banner suppress | `exec "return 1+1;"` | 28 | **2** | 93% | stderr 25B 제거 |
| **B** Compact JSON (small) | `scene info` | 286 | **203** | 29% | indent → compact |
| **B** Compact JSON (large) | `list` | 9,642 | **6,701** | 30.5% | tool catalog (한 줄 6.7KB) |
| **C** Stacktrace user | `exec "var x=(string)null; return x.Length;"` (`--stacktrace user`, default) | 728 | **275** | 62% | wrapper/System.Reflection 프레임 추가 필터 후 |
| **C** Stacktrace none | 위와 동일 + `--stacktrace none` | 728 | **180** | 75% | stack_trace 자체 제거 |
| **D** Compile error | `exec "int x = ; int y = ;"` | 380 | **354** | 7% | message 축약 + indent 제거 영향 합산. CS1525 중복 두 줄 → 한 줄 + `(+1 more)` |
| **E** Unity Object shallow (default) | `exec "return GameObject.Find('Canvas').transform;"` | 14,456 | **60** | **99.6%** | depth=3 reflection → `{name, type, instanceID}` |
| **E** depth=3 explicit (opt-in) | 위와 동일 + `--depth 3` | 14,456 | **9,147** | 36.7% | compact JSON만 적용 (정보량 유지) |
| **G** Console default 20 | `console --type log` (100개 로그 상태) | 218,069 | **41,785** | **80.8%** | `--lines` 미지정 → 20개 cap |
| **G** Console unlimited | `console --type log --lines 0` | 218,069 | **218,069** | 0% | 옛 무제한 동작 명시 복원 |

**BEFORE 측정 방법**:
- A/B는 동일 바이너리에서 `HERA_AGENT_QUIET=0`(human mode)으로 강제하여 옛 출력 형식을 재현.
- C/D/E는 v0.0.23의 옛 동작을 재현하는 옵션 조합(`--stacktrace full` + `--depth 3`)을 통해 옛 응답 사이즈를 직접 측정.
- G는 `--lines 0`이 옛 무제한 동작과 정확히 일치.

**합산 효과 예시 (디버깅 세션 — 한 턴 안에서 다중 호출)**:
- exec×3 (primitive 반환) + scene info×1 + list×1 + console×1 (로그 가득) + exec×1 (Transform 반환) + exec×1 (NullRef 발생):
  - BEFORE: 28+28+28 + 286 + 9,642 + 218,069 + 14,456 + 728 ≈ **243,265 bytes**
  - AFTER: 2+2+2 + 203 + 6,701 + 41,785 + 60 + 275 ≈ **49,030 bytes**
  - **절감 약 79.8% (≈48,600 토큰)**

**lean 세션 예시 (이상적 사용)**:
- exec×10 (전부 `return null;`):
  - BEFORE: 28×10 = 280 bytes
  - AFTER: 3×10 = 30 bytes
  - **절감 약 89% (≈62 토큰)** — banner 25B × 10 = 250 bytes

---

## 5. 클린업

측정 중 생성된 GameObject (Canvas, UIButtonContainer, UIButton_*, Root, Dummy_*, Canvas2, Root2 등)를 씬에서 제거:

```bash
hera-agent exec "
var names = new[] { \"Canvas\", \"UIButtonContainer\", \"Root\", \"Canvas2\", \"UIButtonContainer2\", \"Root2\" };
foreach (var n in names) {
    var go = GameObject.Find(n);
    if (go != null) GameObject.DestroyImmediate(go);
}
return null;
"
```

또는 씬을 reload:

```bash
hera-agent scene load Home --mode single
```

---

## 6. 메모

- **Debug.Log은 stdout 안 옴**: ExecuteCsharp는 사용자 코드의 Debug.Log를 그대로 Unity Console로 보내고 응답에 포함하지 않음. 따라서 디버깅용 Debug.Log을 자유롭게 사용해도 응답 토큰 비용은 0. 단 후속 `hera-agent console` 호출이 비용을 만듦.
- **`[hera-agent] compiling...`은 stderr**: 캐시 hit 여부와 무관하게 출력. 개선안 항목 A의 대상.
- **csc 콜드 스타트**: 새 Unity 세션의 첫 exec는 csc 컴파일러 자체 로딩에 5~15초. 두 번째 호출부터는 ExecCompileCache로 빠름. 본 벤치 측정값은 모두 warm 상태.
- **병렬 exec 주의**: Unity 메인 스레드 직렬화로 인해 여러 hera-agent exec을 병렬 실행하면 큐잉되며, 측정 중 어떤 조합에서 deadlock 비슷한 무한 대기가 관찰됨 (재현 조건 미특정). 본 벤치는 **순차 실행** 권장.

---

## 7. 변경 이력

| 날짜 | 버전 | 변경 |
|------|------|------|
| 2026-05-20 | v0.0.23 (BASELINE) | 초기 측정 — naive/lean/batched 5 시나리오 |
| 2026-05-20 | CLI dev / Connector 0.0.20 (AFTER) | §4 + §4.1 AFTER 측정 채움 — A/B/C/D/E/G 항목 모두 검증. 디버깅 세션 누적 79.8% 절감 확인. |
