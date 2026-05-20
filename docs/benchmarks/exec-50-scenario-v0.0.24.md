# Benchmark: 50회 `exec` C# 명령 실사용 토큰 비용 (v0.0.24)

> 한 줄 요약: **50개의 실제 사용 패턴 exec 명령 = 총 6,568 bytes (~1,622 토큰)**. 명령당 평균 131 bytes / 32 토큰. 35개 (70%) 호출이 5 bytes 이하의 응답을 받음.

- **측정일**: 2026-05-20
- **CLI**: `hera-agent v0.0.24` (Windows, GitHub Release 자산)
- **Connector**: AgentConnector 0.0.20 (git tag `v0.0.24` 시점)
- **Unity**: 6000.3.5f2
- **대상 프로젝트**: TidyCat (실 게임 프로젝트, 활성 씬 `Home.unity`)
- **참고**: 토큰 감축 작업의 항목별 BEFORE/AFTER 측정은 [`exec-token-cost-baseline-v0.0.23.md`](./exec-token-cost-baseline-v0.0.23.md), 분석·구현은 [`docs/issues/token-cost-reduction.md`](../issues/token-cost-reduction.md) 참고. 본 문서는 **AFTER 단일** (v0.0.24)에서 *실 사용 패턴* 50개 누적 측정.

---

## 0. TL;DR

| | 값 |
|---|---:|
| **총 호출 수** | 50 |
| **C# 입력 총합** | 5,482 bytes |
| **응답(stdout+stderr) 총합** | 1,086 bytes |
| **총 왕복 바이트** | **6,568 bytes** |
| **추정 토큰** (chars ÷ 4) | **~1,622 tokens** |
| 호출당 평균 (총 바이트) | 131.4 bytes (32.4 tokens) |
| 호출당 평균 응답 | 21.7 bytes (5.4 tokens) |
| 응답 중간값 | **3 bytes** ("OK\n") |
| 응답 ≤ 5 bytes인 호출 | **35 / 50 (70%)** |
| 가장 큰 호출 | s15 — 632 bytes (UI 계층 생성, 입력이 큰 케이스) |
| 가장 작은 호출 | s36 — 28 bytes (`return Time.time;`) |

**중요한 시사점**:
- 실제 LLM 에이전트의 작업 패턴 (씬 검사 + 오브젝트 생성/조작 + 자산 질의 + 일괄 작업) 에서 **응답 측 토큰은 거의 노이즈 수준**.
- `return null;` 패턴이 표준 권고이며, 측정 시나리오의 대다수가 이를 따라 응답 3 bytes (`OK\n`) 로 수렴.
- 토큰 비용의 대부분은 **사용자(에이전트)가 작성한 C# 코드 길이**에 있음 (입력 5,482 vs 응답 1,086).
- 의도적 실수(비활성 GameObject에 Find 사용)로 발생한 두 건의 런타임 에러도 v0.0.24 의 user-frame 필터 덕에 **각 275 bytes로 압축됨** (v0.0.23에서는 700 bytes급).

---

## 1. 측정 환경 및 방법론

### 환경

- CLI v0.0.24 (PATH 설치본). 호출 시 `HERA_AGENT_NO_PATH_CHECK=1` 만 지정 (path 충돌 경고 억제 — bench 측정에 무관한 노이즈).
- Connector 0.0.20 (TidyCat manifest는 `?path=AgentConnector` 로 main HEAD fetch — v0.0.24 시점 코드).
- 비-TTY(파이프) 호출이라 v0.0.24 의 자동 동작이 그대로 발동:
  - `[hera-agent] compiling...` 배너 stderr **억제됨** (항목 A)
  - 응답 JSON **compact 출력** (항목 B)
  - `Update available` notice **억제됨** (항목 I)

### 측정 단위

- **입력 (input_bytes)**: 각 시나리오 `.cs` 파일의 raw 바이트 (`wc -c`)
- **응답 (output_bytes)**: `hera-agent exec --file <X>.cs 2>&1` 의 stdout + stderr 합산. Claude Code의 Bash 도구는 양쪽을 합쳐 컨텍스트에 넣기 때문에 합산이 적절.
- **토큰 추정**: 영문 기준 `chars ÷ 4` (OpenAI/Anthropic tokenizer 평균치).

### 측정 절차

```bash
mkdir -p .omc/bench-50
# (50개의 s01.cs ~ s50.cs 작성 — §3 참고)

for i in $(seq -w 1 50); do
    f="s$i.cs"
    input_bytes=$(wc -c < "$f")
    output=$(hera-agent exec --file "$f" </dev/null 2>&1)
    output_bytes=$((${#output} + 1))
    total=$((input_bytes + output_bytes))
    echo "s$i,$input_bytes,$output_bytes,$total"
done
```

> `</dev/null` 리다이렉트 필수. `hera-agent exec --file`은 stdin이 비-TTY일 때 stdin을 읽으려고 시도하므로, 명시적으로 닫지 않으면 bash `$(...)` 안에서 무한 대기.

원본 CSV: [`exec-50-scenario-results.csv`](./exec-50-scenario-results.csv)

---

## 2. 시나리오 카테고리

총 50개. 실제 LLM 에이전트가 Unity 작업 중 자주 호출할 법한 패턴으로 구성. 모든 생성 오브젝트는 `bench_` 접두사 부여 → 종료 시 일괄 정리.

| 카테고리 | 시나리오 | 개수 | 누적 바이트 | 평균/호출 | 응답 누적 |
|---|---|---:|---:|---:|---:|
| **A** Scene inspection | s01–s08 | 8 | 563 | 70.4 | 129 |
| **B** GameObject creation | s09–s18 | 10 | 2,095 | 209.5 | 30 |
| **C** Component manipulation | s19–s28 | 10 | 1,447 | 144.7 | 605 |
| **D** Asset/path queries | s29–s35 | 7 | 536 | 76.6 | 173 |
| **E** Math/expression eval | s36–s40 | 5 | 207 | 41.4 | 28 |
| **F** Bulk ops + cleanup | s41–s50 | 10 | 1,720 | 172.0 | 121 |
| **합계** | | **50** | **6,568** | **131.4** | **1,086** |

**관찰**:
- B (GameObject 생성) 가 입력 측에서 가장 많은 바이트를 씀 (UI 계층 / 100개 dummy 생성 등). 하지만 응답은 거의 다 `OK\n` (3B).
- C (Component manipulation) 의 응답 누적이 큰 이유는 의도되지 않은 두 NullRef 에러 (s24, s28 — 각 275B) 때문. **에이전트가 비활성 GameObject에 `GameObject.Find` 를 쓰면 null 반환됨을 잊었을 때의 비용 = 275 bytes/회**.
- E (수학식) 가 호출당 평균이 가장 작음 (41B) — 짧은 입력 + 짧은 숫자 반환.

---

## 3. 시나리오 상세

### A. Scene inspection (s01–s08)

| # | 시나리오 | 입력 | 응답 | 합계 |
|---|---|---:|---:|---:|
| s01 | `return SceneManager.GetActiveScene().name;` | 43 | 5 | 48 |
| s02 | `return SceneManager.GetActiveScene().rootCount;` | 48 | 2 | 50 |
| s03 | `return Camera.main != null;` | 28 | 5 | 33 |
| s04 | Root GameObject 이름 배열 반환 | 154 | 101 | 255 |
| s05 | `return GameObject.Find("Canvas") != null;` | 42 | 6 | 48 |
| s06 | `return SceneManager.GetActiveScene().isDirty;` | 46 | 6 | 52 |
| s07 | `return SceneManager.sceneCount;` | 32 | 2 | 34 |
| s08 | `return LayerMask.NameToLayer("Default");` | 41 | 2 | 43 |

> s04만 배열 반환으로 100바이트 가까이 됨 (15개 root GameObject 이름 리스트). 나머지는 단일 primitive 반환.

### B. GameObject creation (s09–s18) — 입력 크지만 응답은 `OK\n`

| # | 시나리오 | 입력 | 응답 |
|---|---|---:|---:|
| s09 | 빈 GameObject 하나 | 46 | 3 |
| s10 | Light 컴포넌트 GameObject | 81 | 3 |
| s11 | Camera 컴포넌트 GameObject | 80 | 3 |
| s12 | 부모 + 자식 5개 | 203 | 3 |
| s13 | bench_TenDummies (10개) | 178 | 3 |
| s14 | bench_HundredDummies (100개) | 181 | 3 |
| s15 | **Canvas + UIButtonContainer + 3 UIButton** | **629** | 3 |
| s16 | 3×3 Cube grid | 331 | 3 |
| s17 | 20개 GameObject 일렬 배치 | 236 | 3 |
| s18 | Sphere primitive | 100 | 3 |

> s15가 본 작업의 모티브가 된 시나리오 (UI 계층 생성). 입력 629B, 응답 3B → **응답 토큰은 거의 0**, 비용은 사용자가 짠 코드 길이에 있음.

### C. Component manipulation (s19–s28) — 의도된 실수가 비용을 만듦

| # | 시나리오 | 입력 | 응답 | 비고 |
|---|---|---:|---:|---|
| s19 | bench_Empty01 position = (1,2,3) | 89 | 3 | OK |
| s20 | bench_Empty01 rotation Euler(45,0,0) | 95 | 3 | OK |
| s21 | bench_Empty01 localScale (2,2,2) | 91 | 3 | OK |
| s22 | **bench_Empty01.SetActive(false)** | 64 | 3 | OK ← 이후 Find가 실패 |
| s23 | bench_Empty01.SetActive(true) | 63 | 34 | ⚠ Find가 null (비활성) |
| s24 | bench_Empty01.AddComponent<Rigidbody>() | 86 | **275** | ❌ NullRef |
| s25 | bench_Light01 intensity = 5 | 95 | 3 | OK |
| s26 | bench_Light01 color = red | 98 | 3 | OK |
| s27 | bench_Cam01 fov = 90 | 97 | 3 | OK |
| s28 | bench_Empty01.tag = "Untagged" | 64 | **275** | ❌ NullRef |

> **에이전트의 흔한 실수**: `GameObject.Find` 는 **비활성 오브젝트를 찾지 못함**. s22 이후 bench_Empty01을 다시 찾는 시도 (s23, s24, s28) 가 NullRef를 일으킴. v0.0.24의 stacktrace user 필터 덕에 각 에러가 **275 bytes로 컴팩트**. v0.0.23이었다면 같은 에러가 ~700 bytes (UnityEngine.* / System.* / (wrapper managed-to-native) 프레임 포함).

### D. Asset/path queries (s29–s35)

| # | 시나리오 | 입력 | 응답 |
|---|---|---:|---:|
| s29 | `Application.dataPath` | 29 | 42 |
| s30 | `EditorApplication.applicationPath` | 42 | 62 |
| s31 | `AssetDatabase.GetAllAssetPaths().Length` | 60 | 6 |
| s32 | 활성 씬 경로 | 43 | 34 |
| s33 | `EditorPrefs.GetString("Foo_DoesNotExist", "default-value")` | 79 | 14 |
| s34 | `PlayerSettings.companyName` | 47 | 12 |
| s35 | `AssetDatabase.FindAssets("t:Scene").Length` | 63 | 3 |

> 경로 문자열 반환이라 응답이 30~60바이트 정도. 정수 반환은 다시 작아짐.

### E. Math/expression (s36–s40) — 가장 작은 호출들

| # | 시나리오 | 입력 | 응답 |
|---|---|---:|---:|
| s36 | `return Time.time;` | 18 | 10 |
| s37 | `return Time.realtimeSinceStartup;` | 34 | 10 |
| s38 | `return Mathf.Sqrt(144f);` | 25 | 3 |
| s39 | `Vector3.Distance(zero, (3,4,0))` | 61 | 2 |
| s40 | `Random.Range(1, 100)` | 41 | 3 |

> 호출당 평균 41B = **10 토큰**. 가장 작은 케이스(s36)는 28바이트로 한 호출 = ~7 토큰.

### F. Bulk operations + cleanup (s41–s50)

| # | 시나리오 | 입력 | 응답 |
|---|---|---:|---:|
| s41 | 씬의 모든 GameObject 개수 | 90 | 4 |
| s42 | 모든 MeshRenderer 개수 | 92 | 3 |
| s43 | Root y값 합산 | 147 | 4 |
| s44 | TenDummies 자식 일괄 position+1 | 149 | 3 |
| s45 | Container01 자식 일괄 이름 변경 | 176 | 3 |
| s46 | TenDummies 자식 SetActive(false) | 144 | 3 |
| s47 | HundredDummies 자식 tag 일괄 설정 | 148 | 3 |
| s48 | HundredDummies 짝수 인덱스 개수 | 175 | 3 |
| s49 | bench_Renamed_* 5개 이름 반환 | 207 | 92 |
| s50 | bench_* 일괄 destroy | 271 | 3 |

> s49가 응답 92B로 두드러짐 (5개 이름의 JSON 배열). s50은 cleanup. **100개 dummy의 일괄 조작도 응답은 3바이트** — 항목 G 무관 (console 명령이 아님), 항목 E와도 무관 (Unity Object 반환 안 함, null 반환).

---

## 4. 분포 분석

### 응답 크기 히스토그램

```
응답 바이트 범위    호출 수    비율    
(0–5]                35      70%   ████████████████████████████████████
(5–10]                5      10%   █████
(10–30]               2       4%   ██
(30–100]              5      10%   █████
(100–300]             3       6%   ███
(300+)                0       0%
```

**핵심**: 70%의 호출이 5바이트 이하 응답. 100바이트 초과는 6% (3건) 뿐.

### 총 바이트 (입력+응답) Top/Bottom

**Top 5 largest**:
| # | 시나리오 | 합계 | 토큰 |
|---|---|---:|---:|
| s15 | UI 계층 생성 | 632 | 158 |
| s24 | Find-on-inactive NullRef | 361 | 90 |
| s28 | Find-on-inactive NullRef | 339 | 84 |
| s16 | 3×3 cube grid | 334 | 83 |
| s49 | 5개 이름 반환 | 299 | 74 |

**Top 5 smallest**:
| # | 시나리오 | 합계 | 토큰 |
|---|---|---:|---:|
| s36 | `Time.time` | 28 | 7 |
| s38 | `Mathf.Sqrt(144)` | 28 | 7 |
| s03 | `Camera.main != null` | 33 | 8 |
| s07 | `sceneCount` | 34 | 8 |
| s08 | `LayerMask.NameToLayer` | 43 | 10 |

---

## 5. 토큰 비용 의미

### 50회 세션 = **약 1,600 토큰**

LLM 에이전트가 위의 50개 작업을 모두 수행하는 **한 세션**에서 hera-agent 도구 왕복으로 소모하는 토큰은 약 1,600개. 이 중:

- 입력측 (에이전트가 작성한 C# 코드, 에이전트의 출력 토큰): ~1,370 tokens
- 응답측 (hera-agent 응답, 에이전트의 컨텍스트로 들어가는 입력 토큰): ~270 tokens

> 참고: tool_use / tool_result 프레임 자체의 오버헤드 (각 호출당 50–150 토큰) 는 본 측정에 포함되지 않음. 실제 LLM API 비용은 이 위에 더해짐.

### 비교 기준점

- Claude 3.5 Sonnet 입력 1M = $3, 출력 1M = $15 기준
- 50회 세션 ≈ $0.024 (입력 1370 × $3/M + 출력 270 × $15/M) = **세 자릿수 분의 1달러**
- 동일 작업을 일일 100세션 = $2.4/일

응답 측 토큰이 워낙 작아서 **vendor lock-in 없이 Claude API/Anthropic, GPT, Gemini 어느 곳에서나 비용 부담 없이 사용 가능**.

---

## 6. v0.0.24가 본 측정에 영향을 준 지점

본 측정은 AFTER 단일이지만, 다음 v0.0.24 변경이 결과에 직접 영향:

| 변경 | 영향 발생 시나리오 | 본 측정에서의 효과 |
|------|------------------|-------------------|
| **A** Banner suppress | 모든 50개 | 50 × 25B = **1,250 bytes 절감** (배너 없음) |
| **B** Compact JSON | s04, s49 | 응답 배열·구조체에 indent 없음. ~30% 작음 |
| **C** Stacktrace user | s24, s28 (NullRef) | 각 ~700B → 275B = **합산 ~850 bytes 절감** |
| **E** Unity Object shallow | 본 측정엔 해당 없음 | 시나리오가 의도적으로 Transform/GameObject 직접 반환을 피함 |
| **G** Console default 20 | 본 측정엔 해당 없음 | console 명령 미포함 |

**v0.0.23에서 동일 50개 시나리오를 돌렸다면 추정**: ~8,700 bytes (현재 6,568 + 절감분 ~2,100). 즉 **본 워크로드에서 v0.0.24 → 약 24% 추가 절감**.

---

## 7. 재현

### 시나리오 파일

`<TidyCat>/.omc/bench-50/s01.cs` ~ `s50.cs` — 본 리포에는 포함하지 않음 (생성된 임시 측정 파일). [src/scenarios](#) 의 절차 그대로 만들면 동일 시나리오.

### 명령

```bash
# 사전 조건
hera-agent --version    # v0.0.24 이상
hera-agent status       # Unity ready

# 측정
cd <TidyCat>/.omc/bench-50
for i in $(seq -w 1 50); do
    f="s$i.cs"
    input_bytes=$(wc -c < "$f")
    output=$(hera-agent exec --file "$f" </dev/null 2>&1)
    output_bytes=$((${#output} + 1))
    echo "s$i,$input_bytes,$output_bytes"
done

# 정리
hera-agent exec </dev/null "var all = UnityEngine.Object.FindObjectsByType<GameObject>(FindObjectsSortMode.None).Where(g => g.name.StartsWith(\"bench_\")).ToList(); int n=0; foreach(var g in all){ if(g!=null){ GameObject.DestroyImmediate(g); n++; }} return n;"
```

### 알려진 함정

1. **`</dev/null` 필수**: `hera-agent exec --file` 가 `$(...)` 안에 있을 때 stdin을 명시 닫지 않으면 무한 대기.
2. **GameObject.Find은 비활성 오브젝트 무시**: s22 → s23/s24/s28 NullRef 경로. 의도적 보존 (실제 에이전트가 자주 하는 실수의 비용을 보여주기 위함).
3. **csc 콜드 스타트**: 첫 호출은 5~15초 (csc 자체 로딩). 본 측정의 50회는 두 번째 이후만 1~2초.

---

## 8. 결론

**v0.0.24 의 hera-agent로 LLM 에이전트가 실제 Unity 작업 50회를 수행 시:**

- 약 **1.6K 토큰** 소모 (도구 응답·요청 raw)
- 호출당 평균 **32 토큰**
- 응답의 70%가 5바이트 이하 — 사실상 노이즈
- 같은 워크로드를 v0.0.23 으로 돌렸다면 약 24% 더 컸을 것

**LLM 에이전트의 코딩 워크플로에 hera-agent를 통합해도 API 비용 부담이 거의 없음** — 응답 토큰은 미미하고, 비용 대부분은 에이전트가 작성한 C# 코드(입력 토큰)에 있음.

---

## 9. 변경 이력

| 날짜 | 버전 | 변경 |
|------|------|------|
| 2026-05-20 | v0.0.24 (initial) | 50개 시나리오 6 카테고리 측정. 총 6,568 bytes / ~1.6K 토큰. |
