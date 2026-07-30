# WeKnora Wiki 全量评测复盘（2026-07-29）

本文记录上次有效全量评测 `run_20260729_182526` 的调用方式、执行链路和结果结论。目的是让后续接手的人能判断：评测到底打到了哪个服务、如何入库、如何增添、如何删除，以及删除链路上的等待逻辑是否真的避免了采样竞态。

## 结论

评测系统通过 `wiki_eval/eval_weknora.py` 使用 Python `requests.Session` 调 WeKnora REST API。上次有效评测是在 WSL 仓库 `/home/liusz10/wiki/WeKnora-fork` 中执行的，命令里的服务地址是 `http://localhost:8080`，API 前缀是 `/api/v1`，所以请求打到的是 WSL 当前运行的后端服务：

- backend: `http://localhost:8080`
- health: `GET http://localhost:8080/health`
- API: `http://localhost:8080/api/v1/...`

它不是直接调用 Go 内部 service，也不是直接读写数据库；入库、更新、删除、wiki 采样全部通过后端 API 完成。后端再负责落库、投递/执行 wiki 生成任务、调用模型与重建链接。

## 上次有效运行

有效 run：

```text
/home/liusz10/wiki/WeKnora-fork/wiki_eval/reports/run_20260729_182526
```

对应日志：

```text
/home/liusz10/wiki/WeKnora-fork/.local-run/wiki_eval_full_20260729_182526.log
```

逻辑命令：

```bash
python3 -B wiki_eval/eval_weknora.py \
  --base-url http://localhost:8080 \
  --dataset wiki_eval/datasets/stardust \
  --reports-dir wiki_eval/reports \
  --run-update \
  --run-delete \
  --timeout-sec 900 \
  --interval-sec 10
```

认证使用本地环境变量：

```text
WEKNORA_API_KEY
WEKNORA_API_KEY_HEADER=X-API-Key
```

模型选择通过创建 KB 时注入模型 ID：

```text
WEKNORA_EMBEDDING_MODEL_ID
WEKNORA_SUMMARY_MODEL_ID
WEKNORA_WIKI_SYNTHESIS_MODEL_ID
```

注意：这里的 `WEKNORA_API_KEY` 是 WeKnora REST API 访问 key，不是模型供应商 key；模型调用所需的供应商 key 由后端已有模型配置负责。

## 评测如何调用 API

`eval_weknora.py` 里的客户端类 `C` 负责拼 URL 和发请求：

- `base` 来自 `--base-url` 或 `WEKNORA_BASE_URL`
- `prefix` 来自 `--api-prefix` 或 `WEKNORA_API_PREFIX`，默认 `/api/v1`
- API 请求 URL 形如 `{base}{prefix}/{path}`
- `/health` 是非 API 前缀请求，直接访问 `{base}/health`

请求头来自：

- `Authorization: Bearer <WEKNORA_TOKEN>`，如果设置了 token
- `<WEKNORA_API_KEY_HEADER>: <WEKNORA_API_KEY>`，上次是 `X-API-Key`

主要端点如下：

| 阶段 | 方法与路径 | 用途 |
|---|---|---|
| 健康检查 | `GET /health` | 确认目标后端可访问 |
| 创建 KB | `POST /api/v1/knowledge-bases` | 创建评测专用知识库，并注入模型 ID |
| 手工入库 | `POST /api/v1/knowledge-bases/{kb}/knowledge/manual` | 把 Stardust Markdown 文档作为 knowledge 入库 |
| 等待入库 | `GET /api/v1/knowledge/{knowledge_id}` | 轮询 knowledge 状态 |
| 等待 wiki | `GET /api/v1/knowledgebase/{kb}/wiki/pages?limit=500` | 轮询 wiki 页面数量 |
| 等待任务 | `GET /api/v1/knowledgebase/{kb}/wiki/stats` | 观察 pending tasks / active 状态 |
| 页面采样 | `GET /api/v1/knowledgebase/{kb}/wiki/pages` | 获取页面列表 |
| 页面详情 | `GET /api/v1/knowledgebase/{kb}/wiki/pages/{slug}` | 获取页面内容、入链、出链、source refs |
| 图谱采样 | `GET /api/v1/knowledgebase/{kb}/wiki/graph` | 采集 wiki graph |
| 搜索采样 | `GET /api/v1/knowledgebase/{kb}/wiki/search` | 计算 Recall@k / MRR |
| 链接健康 | `GET /api/v1/knowledgebase/{kb}/wiki/lint` | 采集 lint 问题 |
| Issue 采样 | `GET /api/v1/knowledgebase/{kb}/wiki/issues` | 采集 wiki issue |
| 删除文档 | `DELETE /api/v1/knowledge/{knowledge_id}` | 触发 delete / retract |
| 自动修复 | `POST /api/v1/knowledgebase/{kb}/wiki/auto-fix` | 删除后做恢复性对照 |

## 入库流程

初始入库会创建一个新 KB，然后读取：

```text
wiki_eval/datasets/stardust/docs_v1/*.md
```

每个 Markdown 文件都会调用一次：

```http
POST /api/v1/knowledge-bases/{kb}/knowledge/manual
```

请求体形如：

```json
{
  "title": "doc01_project_brief.md",
  "content": "...Markdown content...",
  "status": "publish"
}
```

后端返回 `knowledge_id` 后，评测进入 `wait_wiki`：

1. 轮询 `GET /knowledge/{knowledge_id}` 看 `pending/finalizing/completed/failed` 等状态。
2. 轮询 `GET /knowledgebase/{kb}/wiki/pages?limit=500` 看页面数量。
3. 轮询 `GET /knowledgebase/{kb}/wiki/stats` 看 wiki 后台任务状态。
4. 当页面数连续稳定且相关 knowledge 不再处于 `processing/finalizing` 时，认为入库和 wiki 生成稳定。

上次初始入库结果：

- KB: `b07adefe-662a-4ce7-97f4-39eb82734550`
- docs_v1: 8 个文档
- wait: 12 次 poll 后 stable
- `actual_page_count`: 20
- `issues_count`: 0

## 增添流程

增添阶段复用初始 KB，继续读取：

```text
wiki_eval/datasets/stardust/docs_v2/*.md
```

同样通过 `POST /knowledge-bases/{kb}/knowledge/manual` 入库。上次新增 3 个文档：

- `doc03_psionic_engine_spec_v2.md`
- `doc09_nightfall_update.md`
- `doc10_northstar_lc22_notice.md`

之后再次 `wait_wiki`，并生成独立报告：

```text
/home/liusz10/wiki/WeKnora-fork/wiki_eval/reports/run_20260729_182526_after_update
```

关键结果：

- wait: 13 次 poll 后 stable
- `actual_page_count`: 20
- `update_new_fact_term_coverage`: 1.0
- `update_stale_term_absence`: 1.0
- `issues_count`: 0

这说明新增事实被评测命中，且评测定义里的旧术语残留检查通过。

## 删除流程

删除阶段没有复用初始 KB，而是创建一个独立删除回归 KB：

```text
2c8b777e-b20d-4d61-b582-5c91feda9e13
```

这样做可以避免 delete case 污染前面的初始/增添评测结果。删除 KB 会重新导入 `docs_v1`，等 wiki 稳定后，按 `gold/delete_events.json` 执行 3 个删除事件：

- `d001`: `doc05_borealis_incident.md`
- `d002`: `doc08_aurora_beacon_notes.md`
- `d003`: `doc06_review_board_minutes.md`

每个 case 的核心步骤：

1. 删除前采样全量页面详情，记录 `source_refs`、`in_links`、`out_links`。
2. 调用 `DELETE /api/v1/knowledge/{target_id}`。
3. `wait_deleted` 等目标 knowledge 不再出现在相关页面引用里。
4. `wait_delete_links_reconciled` 继续轮询页面，直到必须移除的 `in_links` 和 stale backlink 都清干净。
5. 调用 `POST /api/v1/knowledgebase/{kb}/wiki/auto-fix` 做对照验证。
6. 再次 `DELETE /knowledge/{target_id}`，验证重复删除/回收的幂等性。

删除报告：

```text
/home/liusz10/wiki/WeKnora-fork/wiki_eval/reports/run_20260729_182526_after_delete
```

关键结果：

| 指标 | 结果 |
|---|---:|
| `delete_source_ref_cleanup_rate` | 1.0000 |
| `delete_must_remove_source_refs_rate` | 1.0000 |
| `delete_leak_strip_rate` | 1.0000 |
| `delete_must_remove_in_links_rate` | 1.0000 |
| `delete_stale_inlink_count` | 0.0000 |
| `delete_stale_inlink_page_rate` | 0.0000 |
| `delete_autofix_must_remove_in_links_rate` | 1.0000 |
| `delete_autofix_stale_inlink_count` | 0.0000 |
| `delete_expected_deleted_page_absence_rate` | 1.0000 |
| `delete_idempotent_retract` | 1.0000 |

各 case：

| Case | must_remove_in_links | stale_inlinks | autofix_must_remove_in_links | idempotent |
|---|---:|---:|---:|---|
| `d001` | 1.0 | 0 | 1.0 | true |
| `d002` | 1.0 | 0 | 1.0 | true |
| `d003` | 1.0 | 0 | 1.0 | true |

## 关于采样竞态

之前 `d001` 出现过 `must_remove_in_links = 0.0` 的假阴性。原因不是后端最终状态错误，而是评测在后端异步链接重建完成前提前采样。

上次有效评测已经验证新等待逻辑能避免这个问题。日志里 `d003` 明确出现过中间态：

```text
[wait-delete-links] poll=1 must_remove_in_links=0.0 stale_inlinks=5
[wait-delete-links] poll=2 must_remove_in_links=0.0 stale_inlinks=5
[wait-delete-links] poll=3 must_remove_in_links=1.0 stale_inlinks=0
```

评测没有在 poll 1 或 poll 2 直接记失败，而是等到链接 reconcile 后再采样。因此这次删除报告里的 `d001/d002/d003` 均为 clean。

## 需要注意的结果解读

这套评测的页面、实体、事实、关系、搜索指标是确定性启发式回归信号，不是人工质量评分。上次初始报告仍有一些召回类指标未满分，例如：

- `entity_slug_recall`: 0.4615
- `entity_name_coverage`: 0.3077
- `fact_expected_page_term_coverage`: 0.3750
- `wiki_search_recall@1`: 0.8333

这些反映的是当前 wiki 生成策略、slug 选择、实体拆分、搜索结果与 gold 期望之间的差距。它们不影响本次删除链路修复结论；删除相关核心指标已经达到 1.0 或 0 stale。

删除报告里的 `delete_false_del_rate` 和 `delete_keep_page_unchanged_rate` 也需要谨慎理解。删除后页面被合并、重写或重建可能是合理行为，所以它们更适合作为趋势观察，不应单独作为失败判据。

## 下次复跑建议

推荐在 WSL 部署仓库里复跑，确保 `localhost:8080` 指向当前 Air 后端：

```bash
cd /home/liusz10/wiki/WeKnora-fork
python3 -B wiki_eval/tools/validate_stardust_corpus.py --dataset wiki_eval/datasets/stardust --strict
python3 -B wiki_eval/eval_weknora.py \
  --base-url http://localhost:8080 \
  --dataset wiki_eval/datasets/stardust \
  --reports-dir wiki_eval/reports \
  --run-update \
  --run-delete \
  --timeout-sec 900 \
  --interval-sec 10
```

运行前确认：

- `GET http://localhost:8080/health` 返回 ok。
- `.env` 或启动环境里存在可用的模型配置。
- `WEKNORA_API_KEY` 是 WeKnora API key，header 为 `X-API-Key`。
- `wiki_eval/reports/` 已被 `.gitignore` 忽略，避免把 raw payload 和运行报告提交。

## 追踪文件

上次有效评测的主要证据文件：

```text
/home/liusz10/wiki/WeKnora-fork/.local-run/wiki_eval_full_20260729_182526.log
/home/liusz10/wiki/WeKnora-fork/wiki_eval/reports/run_20260729_182526/report.md
/home/liusz10/wiki/WeKnora-fork/wiki_eval/reports/run_20260729_182526/metrics.json
/home/liusz10/wiki/WeKnora-fork/wiki_eval/reports/run_20260729_182526_after_update/report.md
/home/liusz10/wiki/WeKnora-fork/wiki_eval/reports/run_20260729_182526_after_update/metrics.json
/home/liusz10/wiki/WeKnora-fork/wiki_eval/reports/run_20260729_182526_after_delete/report.md
/home/liusz10/wiki/WeKnora-fork/wiki_eval/reports/run_20260729_182526_after_delete/metrics.json
```
