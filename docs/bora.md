# Bora

Bora is Boreas's built-in assistant. It explains staging problems, reads code
when connected to Sourcebot, and helps the team plan tickets. You approve any
changes it proposes.

## What you can ask

Ask questions such as “Why did wo-718 fail?”, “Which tasks need attention?” or
“How does checkout calculate shipping?” Bora uses your projects, tasks, builds,
activity, settings, metrics and recent logs, plus [code](#code-through-sourcebot)
when available. Project owners can also ask about members and task grants.

Answers start with the conclusion, include a spoken summary for the phone app,
and list any files read. Bora may add a diagram or ask you to choose between
possible meanings. Replies currently use English or Vietnamese, based on your
question.

## Conversations

Start a conversation about your accessible projects, one project or task, a
build, an alert or a log line. Follow-ups use the last 12 messages.

- Answers stream live and continue saving if you disconnect. Stopping an
  answer discards that turn; a server restart interrupts unfinished answers.
- Conversations are private to you, including from administrators. Losing
  access to a project they read hides them until access returns.
- Delete conversations individually or all at once; there is no automatic
  cleanup. Deleting your account deletes its conversations.
- Search matches word prefixes and ignores accents: `loi bui` finds `lỗi build`.
- With [S3 storage](configuration.md#file-storage-s3), attach up to four images
  per question: JPEG, PNG, WebP or GIF, up to 5 MiB each.

## Bora proposes, you decide

Actions run only when you approve them, using your sign-in and the app's normal
API permissions. Roles allow progressively more actions:

| Role | Available actions |
| ---- | ----------------- |
| `operator` | Start, stop or restart tasks |
| `member` | Also create, delete or edit tasks, including environment variables and shared folders |
| `owner` | Also manage members and task grants, and edit or delete the project |

Only administrators can look up arbitrary accounts or change a project's
registry credential through Bora. Other owners can name only existing project
members or grant holders. The app reports completed actions back to Bora.

After reading code, an image or Sourcebot's agent response in a turn, Bora can
propose only start, stop and restart actions. This limits the damage from
instructions planted in external content.

## Who can use Bora

Bora requires a signed-in project member or administrator; task grants alone
and API tokens cannot be used to ask questions. With Sourcebot configured, only
projects with a matching search context are served.

Each person can run three answers at once, with 20 across the server. Extra
requests are rejected rather than queued.

## Code through Sourcebot

An administrator connects a self-hosted [Sourcebot](https://www.sourcebot.dev)
using `BOREAS_SOURCEBOT_URL` and `BOREAS_SOURCEBOT_API_KEY`. Its free code-search
API is sufficient. Create a
[search context](https://docs.sourcebot.dev/docs/features/search/search-contexts)
named after each project's slug. For project `demo`:

```json
"contexts": {
  "demo": {
    "include": ["github.com/acme/web", "github.com/acme/api"],
    "description": "Demo: web app and API"
  }
}
```

Context changes can take five minutes to appear. Bora searches default branches,
reads other refs, compares commits and checks line history. If CI
[reports the deployed commit](deploy-from-ci.md#3-send-the-commit-too), Bora can
inspect that version and compare deployments. Newly pushed commits may not yet
be indexed by Sourcebot.

Code tools block common secret files, including env files, `*.config`,
`appsettings*.json`, keys, certificates and cloud or Terraform credentials.
This does not guarantee that ordinary source files contain no secrets.

### Ask Sourcebot's agent

Sourcebot's paid Ask feature also lets Bora consult its agent and connected
services such as Jira or Slack. **Unlike code search, this is not confined to
the project:** anyone asking can receive information available to the
Sourcebot API key's user.

Set connector write tools to **Blocked** and read tools to **Allowed** in
Sourcebot; Boreas cannot handle approval requests. This integration uses
Sourcebot's web chat route, which upgrades may break.

## Plan a ticket

Bora produces an [OpenSpec](https://github.com/Fission-AI/OpenSpec) change per
project and a shared test plan, without changing code:

1. Paste a ticket key and description (up to 20,000 characters), and select up
   to five projects.
2. Bora researches their code and asks business questions for up to three
   rounds, then presents decisions labelled as team input or assumptions.
3. Answer or correct them, then confirm. Bora writes requirements, cross-project
   contracts, designs, implementation tasks and test cases.
4. Export each project's change as Markdown for a coding agent. API tokens can
   read exports.

Members of every included project, and administrators, can view the plan.
Those eligible to use Bora across those projects can answer, correct, confirm
or retry it. Only its author or an administrator can stop or delete it.

Plans use code tools, not Sourcebot's agent. Two can run at once. A stop, server
restart or model failure leaves the step failed and retryable; the plan records
why it stopped and, for a manual stop, who stopped it.
[Notifications](notifications.md#who-receives-what) alert eligible participants
when input is needed, the plan is ready or a step fails.

## What reaches the AI model

- Environment variable values are never sent. Recognized passwords, keys and
  tokens in questions, history, tool data and saved answers are replaced with
  `[redacted]`. **Unrecognized secrets can still reach the model provider.**
- Logs, notes, code and other external content are treated as untrusted data,
  not instructions. This is a safeguard, not a reason to share secrets.
- Images are sent **without redaction**, including on follow-ups until a newer
  question supplies images. They remain in your S3 bucket, accessible only to
  their uploader through Boreas, until the conversation is deleted or a bucket
  lifecycle rule removes them.

## Turn Bora on

Use an OpenAI-compatible endpoint supporting streaming and tool calling:

| Variable | Purpose |
| -------- | ------- |
| `BOREAS_AI_BASE_URL` | Model endpoint |
| `BOREAS_AI_MODEL` | Conversation model |
| `BOREAS_AI_API_KEY` | Endpoint key; optional for local servers such as Ollama |
| `BOREAS_AI_PLAN_MODEL` | Ticket-planning model; defaults to `BOREAS_AI_MODEL` |

Both the base URL and model are required. Add Sourcebot for code and S3 for
images; see [Configuration](configuration.md), including how to pass settings
not listed in Compose.

Model requests time out after two minutes, or five for plans; a whole answer
has no overall deadline. Concurrency limits do not cap spending: set a budget
with your provider.

Before choosing a model, run the clarification evaluation from a clone with Go
installed:

```bash
BOREAS_TEST_AI_BASE_URL=https://llm.example.com/v1 BOREAS_TEST_AI_MODEL=your-model \
  BOREAS_TEST_AI_API_KEY=your-key go test ./internal/infra/llm -run TestClarifyEval -v
```
