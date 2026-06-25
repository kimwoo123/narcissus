# FleetBoard

여러 워크트리에서 도는 **Claude 세션 + Git 상태 + Azure DevOps 파이프라인**을
한 화면에서 보는 로컬 웹 대시보드. `localhost`에 띄워두고 한 켠에 켜두면 됩니다.

```
┌ pozzetti  /Users/you/pozzetti ─────────────────────────────────────────────┐
│ feat/admin  ●2 ↑1  ● VSCode            PR !314   ● succeeded #482  ⧉ VSCode  │
│   ✋ 입력대기  Plan admin role feature   특정 유저만 admin 권한을…      12분 전  │
│   ⏳ 작업중   Connect frontend button   백엔드 push 하자               방금     │
└─────────────────────────────────────────────────────────────────────────────┘
```

각 행은 **세션** 하나. 왼쪽부터 상태 → Claude가 자동 생성한 제목(이 세션이 뭐 하던 건지)
→ 마지막 지시 → 마지막 활동 시각. 5초마다 자동 갱신.

## 무엇을 보여주나

| 축 | 출처 | 표시 |
|---|---|---|
| **Claude** | `CLAUDE_HOME/projects/**/*.jsonl` 의 `ai-title`·`last-prompt`·타임스탬프 | 제목 / 마지막 지시 / 상태 |
| **VSCode** | `CLAUDE_HOME/ide/*.lock` 의 `workspaceFolders` | `● VSCode` (현재 열려있음) |
| **Git** | 워크트리별 `git worktree list` + `git status -sb` | `clean` / `●N`(dirty) / `↑↓`(ahead/behind) |
| **Pipeline** | Azure DevOps REST `builds` | `● succeeded/failed #번호` (클릭 → 로그) |
| **PR** | Azure DevOps REST `pullrequests` | `PR !번호` (클릭 → PR) |

### 세션 상태
- `⏳ 작업중` — 최근(기본 45초 내) 활동 = Claude가 응답 생성 중
- `✋ 입력대기` — VSCode 열려있고 마지막이 Claude 차례로 끝났으며 최근(기본 24h 내) = **당신을 기다리는 중**
- `💤 idle` — 그 외

## 실행

```bash
go build -o fleetboard .
./fleetboard
# → http://127.0.0.1:7777
```

## 설정 (전부 환경변수 — 어느 로컬에서나 그대로)

| 변수 | 기본값 | 설명 |
|---|---|---|
| `FLEETBOARD_PORT` | `7777` | 포트 |
| `CLAUDE_HOME` | `~/.claude` | Claude Code 데이터 위치 |
| `FLEETBOARD_MAX_AGE_HOURS` | `168` (7일) | 이 기간 안에 활동한 세션만 표시 |
| `FLEETBOARD_WAITING_HOURS` | `24` | 이보다 오래된 세션은 `입력대기` 대신 `idle` |
| `FLEETBOARD_RUNNING_SEC` | `45` | 이보다 최근 활동이면 `작업중` |
| `FLEETBOARD_REPOS` | — | 세션이 없어도 항상 표시할 레포 경로(`,` 또는 `:` 구분) |
| `ADO_ORG` | — | Azure DevOps organization |
| `ADO_PROJECT` | — | Azure DevOps project |
| `ADO_PAT` | — | Personal Access Token (Build: Read, Code: Read) |

ADO 3종이 비어 있으면 파이프라인/PR 열만 빠지고 나머지는 그대로 동작합니다.

### Azure DevOps 토큰 발급
1. ADO → User settings → **Personal access tokens** → New Token
2. Scopes: **Build (Read)**, **Code (Read)** 체크
3. 발급된 토큰을 환경변수로:

```bash
export ADO_ORG=your-org
export ADO_PROJECT=your-project
export ADO_PAT=xxxxxxxxxxxx
./fleetboard
```

> 토큰은 코드에 박지 않고 `os.Getenv("ADO_PAT")` 로만 읽습니다. shell rc 에 export 해두거나
> `ADO_PAT=… ./fleetboard` 처럼 한 번만 넘기세요.

## 구성

| 파일 | 역할 |
|---|---|
| `config.go` | 환경변수 로딩 |
| `sessions.go` | 세션 jsonl 파싱(+mtime 캐시) |
| `git.go` | 워크트리 / status 파싱 |
| `ado.go` | Azure DevOps REST 클라이언트(+20초 캐시) |
| `aggregate.go` | 레포 → 워크트리 → 세션 집계, 상태 판정 |
| `main.go` | HTTP 서버 (`/` 대시보드, `/api/state` JSON) |
| `web/index.html` | 대시보드 UI (5초 폴링) |

외부 의존성 없음(Go 표준 라이브러리만).

## 다음 단계 (2차)
- 파이프라인 상태 전환 시 **데스크톱 알림** (성공/실패)
- 세션 클릭 → 해당 jsonl 내용 인라인 뷰어
