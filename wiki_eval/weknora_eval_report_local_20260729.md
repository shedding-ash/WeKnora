# WeKnora Wiki 评测报告（local WSL · 2026-07-29）

> 运行环境：本地 WSL 后端 `http://localhost:8080` · 评测框架：`wiki_eval` 确定性回归（不依赖 LLM judge，可复跑） · 语料：Stardust（docs_v1 + docs_v2 增量 + 删除事件）
>
> 有效记录：基线/增量 `run_20260729_182526`，删除 `run_20260729_182526_after_delete`。基线与增量复用 KB，删除回归使用独立 KB，避免污染主流程。

## 1. 核心结论

### 1.1 已验证能力

- **Wiki 生成链路可跑通**：docs_v1 8 篇文档成功入库，12 次 poll 后稳定，`actual_page_count=20`。
- **图谱节点覆盖稳定**：基线与增量的 `graph_node_recall=1.0`，关系端点召回 `relation_endpoint_recall=0.9167`。
- **增量更新有效**：docs_v2 3 篇文档追加后，`update_new_fact_term_coverage=1.0`，`update_stale_term_absence=1.0`，新增事实被吸收，评测定义内旧术语残留已清理。
- **删除/撤回链路已通过关键回归**：`delete_must_remove_in_links_rate=1.0`，`delete_stale_inlink_count=0`，`delete_idempotent_retract=1.0`。此前 d001 的入链残留假阴性在本次有效评测中未复现。
- **AutoFix 覆盖补强有效**：删除后 `delete_autofix_must_remove_in_links_rate=1.0`，`delete_autofix_stale_inlink_count=0`。

### 1.2 关键问题

- **P1 搜索仍有稳定缺口**：`wiki_search_recall@1/3/5=0.8333`，失败用例仍是 `s005`（`Borealis Station Svalbard`）。
- **P1 实体/事实召回未满**：基线 `entity_slug_recall=0.4615`、`entity_name_coverage=0.3077`、`fact_expected_page_term_coverage=0.3750`；增量后分别提升到 `0.6154`、`0.6154`、`0.5000`，但仍不是满分。
- **P2 Lint 问题数量仍高**：基线 `lint_issue_count=311`，增量后 `lint_issue_count=243`。这说明生成内容或链接规范仍有可治理空间。
- **P2 删除保留页稳定性需观察**：删除回归中 `delete_false_del_rate=0.3206`，`delete_keep_page_unchanged_rate=0.5`。该类指标受页面重写/合并影响较大，建议作为趋势信号，不单独作为失败判据。

### 1.3 综合结论

本次评测显示：当前 WeKnora Wiki 的入库、增量更新、删除回收主链路可用；之前阻断级的“删除后入链未清理 / stale backlink 残留”已经修复并由全量回归验证。后续优先级应转向搜索召回、实体 slug 稳定性、事实落页质量和 lint 治理。

## 2. 评测对象与环境

- 系统：WeKnora Go 后端 + Wiki 生成管线
- 运行目录：`/home/liusz10/wiki/WeKnora-fork`
- Windows 主仓库：`D:\rag\myWeKnora\WeKnora`
- 分支：`migration/wiki-eval-and-delete-cleanup`
- 后端地址：`http://localhost:8080`
- API 前缀：`/api/v1`
- 认证：`X-API-Key`
- 评测脚本：`wiki_eval/eval_weknora.py`
- 有效日志：`/home/liusz10/wiki/WeKnora-fork/.local-run/wiki_eval_full_20260729_182526.log`

本次评测通过 HTTP 调用 WSL 后端 API，不直接调用 Go 内部 service，也不直接读写数据库。

## 3. 语料与 Gold

Stardust 语料包含：

- `docs_v1`：8 篇基线文档
- `docs_v2`：3 篇增量文档
- `gold/entities/facts/relations/search/update/delete`：实体、事实、关系、搜索、增量和删除事件标注
- 删除事件：`doc05_borealis_incident.md`、`doc08_aurora_beacon_notes.md`、`doc06_review_board_minutes.md`

语料已通过严格校验：

```bash
python3 -B wiki_eval/tools/validate_stardust_corpus.py --dataset wiki_eval/datasets/stardust --strict
```

## 4. 流程与口径

评测流程为：

```text
健康检查 -> 创建 KB -> docs_v1 手工入库 -> 轮询 Wiki 生成 -> collect 指标
         -> docs_v2 追加入库 -> 轮询 Wiki 更新 -> collect 增量指标
         -> 独立删除 KB -> docs_v1 重新入库 -> 删除事件 -> 等待链接 reconcile -> AutoFix 对照 -> 幂等删除
```

主要 API 口径：

- 建 KB：`POST /api/v1/knowledge-bases`
- 入库：`POST /api/v1/knowledge-bases/{kb}/knowledge/manual`
- 知识状态：`GET /api/v1/knowledge/{knowledge_id}`
- Wiki 页面：`GET /api/v1/knowledgebase/{kb}/wiki/pages`
- Wiki 图谱：`GET /api/v1/knowledgebase/{kb}/wiki/graph`
- Wiki 搜索：`GET /api/v1/knowledgebase/{kb}/wiki/search`
- Wiki lint/issues：`GET /api/v1/knowledgebase/{kb}/wiki/lint`、`GET /api/v1/knowledgebase/{kb}/wiki/issues`
- 删除/撤回：`DELETE /api/v1/knowledge/{knowledge_id}`
- 自动修复：`POST /api/v1/knowledgebase/{kb}/wiki/auto-fix`

指标为确定性字符串/slug 启发式，适合作为回归信号，不等同于最终人工质量评分。

## 5. 摄取与 Wiki 产物

基线 KB：

```text
b07adefe-662a-4ce7-97f4-39eb82734550
```

基线入库结果：

- docs_v1 入库 8 篇
- `wait_wiki` 12 次 poll 后 stable
- `actual_page_count=20`
- `issues_count=0`

增量更新结果：

- docs_v2 追加 3 篇
- `wait_wiki` 13 次 poll 后 stable
- `actual_page_count=20`
- `issues_count=0`

删除回归 KB：

```text
2c8b777e-b20d-4d61-b582-5c91feda9e13
```

删除 KB 重新入库 docs_v1，13 次 poll 后 stable。

## 6. 页面与实体

| 指标 | 基线 | after_update |
|---|---:|---:|
| `actual_page_count` | 20 | 20 |
| `entity_name_coverage` | 0.3077 | 0.6154 |
| `entity_slug_recall` | 0.4615 | 0.6154 |
| `duplicate_like_entity_rate` | 0.0769 | 0.1538 |
| `graph_node_count` | 40 | 47 |

Missed Entities：

- 基线：`skyvault-initiative`、`acme-corporation`、`aurora-beacon`、`mira-cole`、`jonas-reed`、`borealis-station`、`celestial-review-board`
- after_update：`skyvault-initiative`、`acme-corporation`、`psionic-engine`、`mira-cole`、`borealis-station`

增量后实体指标明显提升，但仍存在 slug 选择和实体落页不稳定问题。

## 7. 事实与关系

| 指标 | 基线 | after_update |
|---|---:|---:|
| `fact_expected_page_term_coverage` | 0.3750 | 0.5000 |
| `relation_endpoint_recall` | 0.9167 | 0.9167 |
| `graph_edge_recall_direct` | 0.8333 | 0.9167 |
| `graph_edge_recall_bridge` | 0.0833 | 0.0000 |
| `graph_edge_recall_heuristic` | 0.9167 | 0.9167 |

Missed Facts：

- 基线：`f002`、`f003`、`f004`、`f005`、`f008`
- after_update：`f002`、`f003`、`f005`、`f008`

关系端点和启发式边召回保持在较好水平；事实指标仍低于理想值，主要问题是 gold 事实未落到期望页面或页面术语不完整。

## 8. 知识图谱

| 指标 | 基线 | after_update |
|---|---:|---:|
| `graph_node_recall` | 1.0000 | 1.0000 |
| `graph_node_count` | 40 | 47 |
| `graph_edge_count` | 183 | 351 |
| `graph_edge_recall_direct` | 0.8333 | 0.9167 |
| `graph_edge_recall_heuristic` | 0.9167 | 0.9167 |

图谱结构在增量后扩张明显，节点召回保持满分，直接边召回提升。边数量增加较大，需要结合 lint 和页面内容继续观察是否有过度链接或重复实体扩张。

## 9. 检索

| 指标 | 基线 | after_update |
|---|---:|---:|
| `wiki_search_recall@1` | 0.8333 | 0.8333 |
| `wiki_search_recall@3` | 0.8333 | 0.8333 |
| `wiki_search_recall@5` | 0.8333 | 0.8333 |
| `wiki_search_mrr` | 0.8333 | 0.8333 |

持续失败项：

```text
s005: Borealis Station Svalbard
```

建议后续排查该实体的别名、地名词项、向量召回和关键词召回通道。

## 10. 增量更新评测

| 指标 | after_update |
|---|---:|
| `update_new_fact_term_coverage` | 1.0000 |
| `update_stale_term_absence` | 1.0000 |
| `actual_page_count` | 20 |
| `lint_issue_count` | 243 |

增量事件结果：

- `u001`: PE-8 safety threshold is 48 kPa，命中
- `u002`: Nightfall Protocol was conditionally approved on 2032-06-19，命中
- `u003`: LC-22 replaced LC-19，命中

结论：增量文档能被吸收，且评测定义内旧术语残留为 0。

## 11. 删除与撤回评测

| 指标 | 值 | 判定 |
|---|---:|---|
| `delete_expected_deleted_page_absence_rate` | 1.0000 | PASS：目标页移除 |
| `delete_idempotent_retract` | 1.0000 | PASS：重复删除幂等 |
| `delete_must_remove_source_refs_rate` | 1.0000 | PASS：源引用清理 |
| `delete_must_remove_in_links_rate` | 1.0000 | PASS：入链清理 |
| `delete_stale_inlink_count` | 0.0000 | PASS：无 stale backlink |
| `delete_autofix_must_remove_in_links_rate` | 1.0000 | PASS：AutoFix 后入链仍 clean |
| `delete_autofix_stale_inlink_count` | 0.0000 | PASS：AutoFix 后无 stale backlink |
| `delete_false_del_rate` | 0.3206 | WATCH：趋势观察 |
| `delete_keep_page_presence_rate` | 0.8333 | WATCH：部分保留页缺失 |
| `delete_keep_page_unchanged_rate` | 0.5000 | WATCH：页面重写影响 |

逐 case：

| Case | Target | must_remove_in_links | stale_inlinks | autofix_fixed | false_del | idempotent |
|---|---|---:|---:|---:|---:|---|
| `d001` | `doc05_borealis_incident.md` | 1.0 | 0 | 0 | 0.4286 | true |
| `d002` | `doc08_aurora_beacon_notes.md` | 1.0 | 0 | 6 | 0.5333 | true |
| `d003` | `doc06_review_board_minutes.md` | 1.0 | 0 | 0 | 0.0000 | true |

此前旧评测中出现的 `must_remove_in_links=0.0` 和 stale backlinks 残留，本次已恢复为 clean。

## 12. 采样竞态验证

本次评测特别验证了删除后的链接 reconcile 等待逻辑。日志中 `d003` 曾短暂出现中间态：

```text
[wait-delete-links] poll=1 must_remove_in_links=0.0 stale_inlinks=5
[wait-delete-links] poll=2 must_remove_in_links=0.0 stale_inlinks=5
[wait-delete-links] poll=3 must_remove_in_links=1.0 stale_inlinks=0
```

评测没有在 poll 1 或 poll 2 提前采样，而是等待后端异步全库链接清理完成后才记录指标。这说明本次评测结果不再受删除采样竞态影响。

## 13. 缺陷、风险与改进建议

- **P1 搜索 s005**：定位 `Borealis Station Svalbard` 召回失败原因，重点看别名、地理词项、关键词索引和向量召回。
- **P1 实体 slug 稳定性**：提高 gold 实体到 canonical page 的稳定落点，减少同一实体被 concept/entity/summary 分散承载。
- **P1 事实落页质量**：增强事实在期望实体页上的聚合能力，尤其是 `f002/f003/f005/f008`。
- **P2 Lint 治理**：继续拆解 `lint_issue_count`，区分真实 broken link、重复链接、格式问题和可接受的生成差异。
- **P2 删除保留页语义**：为 `false_del_rate` 和 `keep_page_unchanged_rate` 增加更细的“合理重写”判定，避免把合法合并/重写过度计为风险。

## 14. 附录：原始报告位置

```text
/home/liusz10/wiki/WeKnora-fork/wiki_eval/reports/run_20260729_182526/report.md
/home/liusz10/wiki/WeKnora-fork/wiki_eval/reports/run_20260729_182526/metrics.json
/home/liusz10/wiki/WeKnora-fork/wiki_eval/reports/run_20260729_182526_after_update/report.md
/home/liusz10/wiki/WeKnora-fork/wiki_eval/reports/run_20260729_182526_after_update/metrics.json
/home/liusz10/wiki/WeKnora-fork/wiki_eval/reports/run_20260729_182526_after_delete/report.md
/home/liusz10/wiki/WeKnora-fork/wiki_eval/reports/run_20260729_182526_after_delete/metrics.json
```

本报告基于上述 raw metrics 与 run log 汇总生成。
