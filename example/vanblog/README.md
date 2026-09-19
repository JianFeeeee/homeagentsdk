# vanblog · VanBlog 博客管理

用管理 API 操作 [VanBlog](https://vanblog.mereith.com/) 开源博客系统：
文章增删改查、分类标签、草稿发布、备份导出等。

## 配置项

| 键 | 默认 | 说明 |
|---|---|---|
| `url` | `https://blog.jianfgit.xyz` | VanBlog 站点基地址 |
| `token` | 空 | 管理员 API Token（长期令牌，从后台「Token 管理」创建） |
| `reset_token` | 空 | 用于 `auth/restore` 重置管理员密码的**特殊** Token |

`token` 与 `reset_token` 都是 `password` 类型（界面遮蔽）。

## 工具（28 个）

### 文章

| 工具 | 说明 |
|---|---|
| `vanblog_list_articles` | 列文章，支持分页与搜索 |
| `vanblog_get_article` | 取单篇完整内容 |
| `vanblog_create_article` | 新建（`title` 与 `category` 必填） |
| `vanblog_update_article` | 更新（**只传要改的字段**） |
| `vanblog_delete_article` | 删除（**软删除**） |
| `vanblog_search_articles` | 按链接搜索文章 |

### 草稿

`vanblog_manage_drafts`：`list` / `get` / `create` / `update` / `delete` / **`publish`**

### 内容组织

| 工具 | 命令 |
|---|---|
| `vanblog_manage_categories` | `list` / `get` / `create` / `update` / `delete` |
| `vanblog_manage_tags` | `list` / `get` / `rename` / `delete` |

### 站点与运维

| 工具 | 说明 |
|---|---|
| `vanblog_manage_site` / `_settings` / `_menu` / `_social` / `_links` | 站点配置类 |
| `vanblog_manage_about` / `_pages` | 关于页与自定义页面 |
| `vanblog_manage_images` | 图床管理 |
| `vanblog_manage_rewards` | 赞赏配置 |
| `vanblog_manage_backup` | 备份 |
| `vanblog_manage_caddy` | Caddy 配置 |
| `vanblog_manage_isr` | ISR 增量静态渲染 |
| `vanblog_manage_pipelines` | 流水线 |
| `vanblog_manage_collaborators` | 协作者 |
| `vanblog_manage_tokens` | Token 管理 |
| `vanblog_get_analysis` / `_logs` / `_meta` | 统计、日志、元信息 |
| `vanblog_auth` | 认证相关（含 `restore` 重置密码） |

> 工具名前缀取自插件名（`tp`），上面按默认 `vanblog_` 列出。

## 实现要点

- 走的是 VanBlog 的管理 API（`/api/admin/...`），所以必须配 **admin token**，
  不是前台只读接口。
- `update_article` 是**部分更新**：只传想改的字段，没传的保持不变。
  （不要为了改标题而把正文一起传一遍。）
- `delete_article` 是**软删除**，内容仍在，可在后台恢复。
- 早期版本把 token 放在内核配置（`plugin.vanblog.token`）里，
  现在会**自动迁移**到插件配置，迁移后清空内核侧取值。

## 前置

需要一个可访问的 VanBlog 实例，并在后台创建一个长期 Token。

## 构建

```bash
hmapdev build
```
