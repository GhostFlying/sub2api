# 上游版本自动提升实施计划

参考 GhostFlying/orca 的候选生成、检查和自动 finalizer 流程，沿用本仓库 `fork = upstream-release + fork-only patches` 的维护模型。新版本无需手动 `/promote-fork`，检查通过后自动提升；不移植 Orca 的桌面/移动发布资产体系。

1. 自动同步仅选择已发布的稳定 `vX.Y.Z` Release，定时发现最高版本；指定 tag 也须通过相同校验。保持现有 effective baseline / VERSION 单文件补丁识别、按序 replay 和 Wire 恢复。
2. 生成或复用当前候选时自动安排 finalizer。CI / Security Scan 对 `sync/upstream-*` 的 push 或 workflow_dispatch 检查完成后触发自动 finalizer；提供 workflow_dispatch 作为重试入口，不再依赖 issue_comment。
3. Finalizer 从可信 `fork` checkout 读取维护脚本，只处理同仓库、目标为 `fork` 的稳定版本候选。校验候选 SHA、源 refs marker 和最新的 GitHub Actions 检查；缺失/运行中检查等待，失败检查阻止提升，拒绝外部 fork 和伪造检查。
4. 保留并加强原子 force-with-lease 对 fork、anchor、候选的保护，重新验证 effective baseline 与祖先关系。重复完成事件幂等，候选已被提升时不再次更新生产 refs；删除候选也带租约。
5. 更新 AGENTS.md、fork maintenance runbook、候选 PR / 冲突说明为自动提升。fork push 继续只触发现有 Docker 发布，不额外 dispatch Docker；不执行部署。
6. 新增 Python 标准库维护策略回归测试，覆盖稳定版本选择、精确 SHA、候选来源、marker、检查等待/失败/重复/伪造和幂等边界；CI 运行这些测试。验证 YAML/Bash 语法与相关工作流契约，创建功能 PR 并按已有授权合入 fork。

保持现有安全门槛。已知 fork 的前端依赖安全扫描失败仍阻止相应候选的自动提升，不增加豁免或绕过。完成后验证自动 finalizer 的真实事件路径及 refs 读回；仅在所有门槛满足时才提升。
