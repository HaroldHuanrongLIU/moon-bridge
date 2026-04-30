# 配置迁移

Moon Bridge 还没有公开发布，配置结构变更时会直接切到当前格式，不在运行时保留旧字段别名。旧配置请用迁移脚本做一次性迁移，然后按新结构维护。

---

## v5 迁移（当前格式）

从 v4（含 `provider.providers` 嵌套格式）迁移到 v5（顶层 `providers`/`models`/`routes`）。

迁移脚本：`scripts/migrate_config_v5.py`

### 使用方式

```bash
uv run scripts/migrate_config_v5.py config.yml output.yml
```

建议先跑 `--dry-run` 预览结果（脚本暂不支持 dry-run 标志，可先复制配置做测试）。

### 主要变更

| 旧格式 (v4) | 新格式 (v5) |
|---|---|
| `provider.providers.<key>.models`（客户端别名映射） | 共享模型元数据放顶层 `models.<slug>`，提供商声明放 `providers.<key>.offers[].model` |
| `routes[].to`（如 `"deepseek/deepseek-v4-pro"`） | `routes[].model` + `routes[].provider` |
| `provider.base_url` / `provider.api_key`（顶层） | 删除，改为 `providers.<key>.base_url` / `api_key` |
| `provider.default_model` / `provider.default_max_tokens` / `system_prompt` | `defaults.model` / `defaults.max_tokens` / `defaults.system_prompt` |
| `trace_requests: true` | `trace: { enabled: true }` |
| `developer.proxy.*` | `proxy.*` |

### 示例

v4 格式：

```yaml
provider:
  providers:
    deepseek:
      base_url: "https://api.deepseek.com/anthropic"
      api_key: "sk-xxx"
      models:
        deepseek-v4-pro:
          extensions:
            deepseek_v4:
              enabled: true
  routes:
    moonbridge:
      to: "deepseek/deepseek-v4-pro"
```

迁移后 (v5)：

```yaml
providers:
  deepseek:
    base_url: "https://api.deepseek.com/anthropic"
    api_key: "sk-xxx"
    offers:
      - model: deepseek-v4-pro

models:
  deepseek-v4-pro:
    extensions:
      deepseek_v4:
        enabled: true

routes:
  moonbridge:
    model: deepseek-v4-pro
    provider: deepseek

defaults:
  model: deepseek-v4-pro
  max_tokens: 4096
```

### 注意事项

- 共享模型 slug 在整个配置中必须唯一。如果多个 provider 提供同一个 slug，在 `offers` 中重复引用即可。
- 定价从模型定义层迁移到 `offers[].pricing`，按 provider 分别设置。
- 旧格式中 provider 级的 `web_search` / `extensions` 配置会保留在 provider 定义中。
- 运行迁移脚本前建议备份原配置。

---

# 配置迁移

Moon Bridge 还没有公开发布，配置结构变更时会直接切到当前格式，不在运行时保留旧字段别名。旧配置请用 `scripts/migrate_config.py` 做一次性迁移，然后按新结构维护。

## 使用方式

```bash
python scripts/migrate_config.py --dry-run config.yml
python scripts/migrate_config.py config.yml
python scripts/migrate_config.py old.yml new.yml
```

脚本会保留 YAML 注释和引号，默认原地覆盖输入文件。先跑 `--dry-run` 检查输出，再覆盖真实配置。

## Provider 模型与路由

旧格式把客户端别名放在 `provider.providers.<key>.models` 下面，并用 `name` 写上游模型名：

```yaml
provider:
  providers:
    deepseek:
      models:
        moonbridge:
          name: deepseek-v4-pro
```

v4 格式要求 Provider 模型目录以真实上游模型名为 key，客户端别名单独放到 `provider.routes`：

```yaml
provider:
  providers:
    deepseek:
      models:
        deepseek-v4-pro: {}
  routes:
    moonbridge:
      to: "deepseek/deepseek-v4-pro"
```

## DeepSeek V4

旧的 `provider.deepseek_v4: true`、`provider.providers.<key>.deepseek_v4: true` 或模型级 `deepseek_v4: true` 会迁移到统一 extension 插槽：

```yaml
provider:
  providers:
    deepseek:
      models:
        deepseek-v4-pro:
          extensions:
            deepseek_v4:
              enabled: true
```

## Visual

旧的 `provider.visual: true`、`provider.providers.<key>.visual: true`、模型级 `visual: true` / `enable_visual_extension: true` 会迁移到两层新配置：

```yaml
extensions:
  visual:
    config:
      provider: kimi
      model: kimi-for-coding
      max_rounds: 4
      max_tokens: 2048
provider:
  providers:
    deepseek:
      models:
        deepseek-v4-pro:
          extensions:
            visual:
              enabled: true
    kimi:
      base_url: "https://api.moonshot.ai/anthropic"
      api_key: "replace-with-kimi-api-key"
      models:
        kimi-for-coding: {}
```

迁移脚本会把 `provider.providers.<key>.visual: true` 下推到该 provider 的所有模型，并把旧的模型级 `visual: true` / `enable_visual_extension: true` 改为 `extensions.visual.enabled: true`。旧的全局 `provider.visual: true` 会下推到所有非 Kimi 的 Anthropic 模型，Kimi/Moonshot provider 只作为视觉 provider 使用。如果配置里已有 Kimi/Moonshot provider，会自动填 `extensions.visual.config.provider` 和 `extensions.visual.config.model`；无法推断时会打印 warning，需要手动补齐。

Visual 只支持 Anthropic-routed 主模型和 Anthropic-compatible 视觉 provider。`protocol: "openai-response"` 的模型不能设置 `extensions.visual.enabled: true`，也不能作为 `extensions.visual.config.provider`。
