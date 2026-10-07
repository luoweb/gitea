是否可以支持git@github.com:blockmap/blockmap.github.io.git 协议同步，因为 https 容易被墙

# 结论：**Gitea 原生 WebUI 的【Pull Mirror（拉取镜像）不支持 `git@github.com:...` SSH 格式上游地址**，官方文档明确说明这个限制，表单直接校验不通过，没法直接填这个地址做内置自动镜像Gitea

> 
> 但是可以**外挂脚本 + cron 的方案**，让 Gitea 容器内走 SSH 协议定时拉取 github 仓库，绕过 https 墙的问题。

## 两个方案对比

### 方案 1：外挂独立定时脚本（推荐，适配你 NAS 的 Gitea rootless 容器，SSH 协议拉取 github）

原理：

1. 容器内 gitea 用户（uid=1000）生成 ssh 密钥，把公钥加到 github 账号
2. 写 shell 脚本，`git fetch --mirror` 通过 ssh 地址拉取 `git@github.com:blockmap/blockmap.github.io.git`
3. 宿主机 cron 定时执行脚本，更新 Gitea 仓库；**仓库在 Gitea 里创建为普通仓库，不使用 Gitea 原生镜像功能**

> 
> 优点：走 SSH 协议，规避 HTTPS 被墙；完全可控；
> 缺点：**不再使用 Gitea 自带镜像后台，自己维护同步脚本**，不会自动同步 issue/pr（本来原生镜像也不同步 issue）

#### 详细步骤

1. 进入 gitea 容器，切换 gitea 用户，生成 ssh 密钥

```
# 进入容器
docker exec -u 1000 -it gitea-light bash
# 生成ed25519密钥（不要密码）
ssh-keygen -t ed25519 -N "" -f ~/.ssh/id_ed25519
# 查看公钥，复制
cat ~/.ssh/id_ed25519.pub
```

把公钥添加到你的 GitHub 账号 → Settings → SSH and GPG keys。

2. 在 Gitea 网页**新建普通仓库（不要勾选镜像！）**，仓库名 `blockmap.github.io`
记下仓库在容器内的路径：`/var/lib/gitea/git/repos/admin/blockmap.github.io.git`（admin 换成你的 gitea 用户名）
3. 写同步脚本 `sync-blockmap.sh`，放到宿主机 `/data/gitea/scripts/`

```
#!/bin/bash
REPO_PATH="/var/lib/gitea/git/repos/admin/blockmap.github.io.git"
UPSTREAM="git@github.com:blockmap/blockmap.github.io.git"
# 进入裸仓库
cd "${REPO_PATH}" || exit 1
git remote set-url origin ${UPSTREAM}
git fetch --prune --mirror origin
```

> 
> 赋予权限：

```
chown 1000:1000 /data/gitea/scripts/sync-blockmap.sh
chmod +x /data/gitea/scripts/sync-blockmap.sh
```

4. 宿主机 crontab 定时执行（例如每 10 分钟同步一次）

```
*/10 * * * * docker exec -u 1000 gitea-light /var/lib/gitea/scripts/sync-blockmap.sh >> /var/lib/gitea/scripts/sync.log 2>&1
```

### 方案 2：HTTPS + 容器内配置 git 代理（备选，不用脚本，继续使用 Gitea 原生镜像功能）

> 
> 如果你想**继续使用 Gitea 自带 Pull Mirror**，不自己写 cron 脚本：
> 表单只能填 https 地址，但是在 Gitea 容器内给 git 配置 socks5/http 代理，让 https 流量走代理穿墙访问 github。

容器内 gitea 用户配置 git 全局代理：

```
docker exec -u 1000 -it gitea-light bash
git config --global http.proxy socks5://你的代理IP:端口
git config --global https.proxy socks5://你的代理IP:端口
```

然后 WebUI 镜像地址填写：`https://github.com/blockmap/blockmap.github.io.git`，开启 Gitea 原生镜像。

> 
> 优点：继续使用 Gitea 原生镜像管理页面；缺点：需要常驻代理。

## 补充：Push Mirror（Gitea 推送到外部）的 SSH 限制

Gitea**推送镜像（Push Mirror）同样原生不支持 SSH**，只能填 https；如果想 Gitea 提交后 SSH 推送到 github，官方文档方案是用仓库 `post-receive` 钩子调用 git 命令推送（和方案 1 思路类似）Gitea

## 选型建议

- 不想常驻代理、希望直接 SSH 协议拉取 github：**方案 1（cron + 脚本）**，适合 NAS
- 已有 socks/http 代理，想保留 Gitea 网页镜像管理：**方案 2（git 代理 + https 原生镜像）**

## 额外踩坑提醒

1. rootless 容器，ssh 私钥归属必须是 uid=1000，权限 600，否则 ssh 拒绝使用密钥；
2. GitHub 公钥加到账号，不是加到单个仓库；
3. 裸仓库 `git fetch --mirror` 会同步所有分支和 tag，和 Gitea 原生镜像行为一致；
4. 这种脚本方案仓库是普通仓库，**Gitea 不会标记为只读镜像**，注意不要在 Gitea 这边提交代码，否则会冲突。

你需要我把这个同步脚本改成**批量仓库版本**，后续批量同步多个 github 仓库吗？

昨天 11:14

改造 gitea 支持 ssh git 协议的可行性

# 结论先行

**技术上完全可行，但属于源码层面二次改造，有维护成本；不推荐直接改 Gitea 原生 Pull Mirror，优先外挂脚本方案。**

> 
> Gitea 官方原生限制：Pull Mirror（拉取镜像）Web 表单校验**拒绝 `git@xxx` SSH 格式地址**，底层镜像任务模块最初只设计处理 HTTP/HTTPS 远程；**并不是 git 命令本身不能跑 SSH**，Gitea 调用底层`git`二进制时本身完全支持 SSH 协议，卡点在：**表单校验 + 镜像模型存储 + 认证管理**Gitea。

## 一、为什么原生不支持 SSH 上游镜像（Root Cause）

1. **前端表单 URL 正则校验**
迁移页面有正则，只允许 `http:// / https://`，直接拦截 `git@xxx`，提示不是有效 URL。
2. **镜像数据模型设计**
Pull Mirror 数据库模型设计时，**绑定了 HTTP 基础认证（username/password/token）**；SSH 认证体系是独立的私钥体系，模型没有预留存储 SSH 私钥、known_hosts 的字段。
> 
> HTTPS 用 Token；SSH 用独立私钥，两套认证模型，Gitea 原代码没有处理 SSH 私钥的持久化、选择、管理。
3. **任务执行层面：git 命令本身没问题**
Gitea 后台镜像同步本质是调用系统`git fetch --mirror`。**只要运行 gitea 进程的用户（uid=1000）有可用 ssh 私钥、known_hosts，git 命令本身完全可以拉 git@github 地址**。
> 
> 限制不是 Git，是 Gitea 上层业务代码不支持录入、保存、管理 SSH 私钥给镜像任务。

> 
> 区分两个概念：
> ✅ Gitea**对外提供 SSH 克隆**（你 clone gitea 仓库用 git@xxx）：原生完整支持，这个功能没问题。
> ❌ Gitea**作为镜像客户端，主动用 SSH 协议拉外部上游仓库**：Pull Mirror 模块不支持。

## 二、改造方案分级（由简单→硬核源码改造）

### Level 0：无需改源码（推荐，就是前面说的外挂 cron 脚本）

**不改动 Gitea 任何代码，WebUI 镜像功能不用，仓库创建为普通仓库**

- Gitea 仓库作为普通裸仓库；宿主机 cron 定时执行脚本，容器内 gitea 用户执行`git fetch --mirror git@github.com:xxx.git`
- 自行维护 ssh 私钥、known_hosts；同步完 Gitea 会自动读取裸仓库数据，WebUI 正常展示提交 / 分支 /tag
✅ 优点：零源码编译、升级 Gitea 不影响；NAS 部署适配，稳定。
❌ 缺点：**不再使用 Gitea 内置镜像后台**，Web 上没有「立即同步」按钮，同步日志、任务队列不在 Gitea 管理面板。

### Level 1：最小改动源码（仅绕过表单校验，**不支持多仓库独立 SSH 密钥**）

修改两处代码：

1. 前端表单 URL 校验正则，放行 `git@xxx` 格式地址。
2. 后端迁移 URL 校验逻辑，移除对 http/https 协议强制校验。

限制：**全局共用一套 SSH 私钥**（gitea 用户～/.ssh/id_ed25519），**所有 SSH 镜像仓库共用同一个密钥**，不能每个上游仓库单独绑定不同 SSH 密钥。

> 
> 适合你的场景：全部 github 仓库，统一用一个 GitHub SSH 密钥。
> 构建：拉取 Gitea release 源码，Go 编译，重新打包 rootless docker 镜像。

✅ 优点：WebUI 可以录入 git@地址，走 Gitea 原生镜像 cron 任务，Web 面板可以点「立即同步」，镜像状态存入数据库。
❌ 缺点：密钥全局共用；升级新版本 Gitea，每次都需要重新打补丁、编译镜像；无法给不同上游配置不同 ssh 私钥。

### Level 2：完整深度改造（生产级，支持每个镜像仓库独立 SSH 私钥）

除 Level1 之外，额外改动：

1. 数据库 mirror 表增加字段：ssh_private_key、ssh_known_hosts。
2. 后台任务执行镜像拉取时，**临时为该任务加载对应仓库的私钥**（通过 SSH agent 或者临时 HOME 环境变量隔离不同仓库密钥）。
3. WebUI 新增表单：镜像页面增加 SSH 私钥文本框、known_hosts 输入框。
4. 增加密钥安全校验、权限控制，防止私钥泄露。

✅ 优点：完全原生体验，每个镜像仓库独立 SSH 密钥，和 HTTPS 镜像功能体验一致。
❌ 缺点：改动量大，涉及数据库迁移、前后端大量代码；**后续升级 Gitea 主线版本维护成本很高，每次合并新版本都要解决大量代码冲突**，个人 NAS 场景不划算。

## 三、改造风险评估（重点，NAS 环境）

1. **升级负担**：Gitea 几乎每月有小版本，每次升级，自定义补丁需要手动合并；一旦代码重构，补丁直接失效。官方不会合并这个功能，多年多次 issue 请求都没有并入主线。
2. **SSH 密钥安全**：如果做 Level2，数据库存储私钥，需要加密；一旦配置不当存在私钥泄露风险。Level1 全局密钥风险低，但是密钥复用。
3. rootless 容器额外坑：容器内 gitea 用户（uid=1000）的`.ssh`目录权限必须严格`700`，私钥`600`，否则 git/ssh 拒绝读取密钥，和你之前碰到的 volume 权限坑叠加。

## 四、备选折中方案：不改造代码，Gitea 原生镜像 + 容器 git 代理（不用 SSH 协议）

继续使用 Gitea 原生 Pull Mirror，地址填 https，**容器内 git 配置 socks5 代理**，绕墙访问 github https，不需要改任何代码，保留完整 Web 镜像管理。

> 
> 缺点：需要常驻代理；优点完全原生，升级无负担。

## 五、选型对比（针对你的场景：NAS，批量 github 镜像，规避 https 墙）

表格

| 方案                                | 是否改源码 | Web 镜像面板     | 独立仓库密钥     | 维护成本         | 推荐度 |
| ----------------------------------- | ---------- | ---------------- | ---------------- | ---------------- | ------ |
| 外挂 cron 脚本                      | ❌ 0 改动   | 无内置镜像管理   | 可单独管理密钥   | 低               | ⭐⭐⭐⭐⭐  |
| 最小补丁 (放行 git@，全局 ssh 密钥) | ✅少量补丁  | 完整原生镜像面板 | 全局共用密钥     | 中，升级需重编译 | ⭐⭐⭐⭐   |
| 完整深度改造（独立 ssh 密钥）       | ✅大量改动  | 完整原生面板     | 每个仓库独立密钥 | 极高             | ⭐⭐     |
| 原生 HTTPS+git 代理                 | ❌0 改动    | 完整原生面板     | 不需要 ssh 密钥  | 低，依赖代理     | ⭐⭐⭐⭐   |

## 六、我的建议

你的场景（NAS、批量 github 仓库镜像）：**优先 Level0 外挂脚本方案，不要去源码改造 Gitea**。
源码改造适合深度二次开发项目；个人 NAS 做 github 镜像备份，外挂脚本性价比最高，稳定，升级 Gitea 不受影响。

如果你想要，我可以：

1. 写批量同步脚本，一次性管理多个 github 仓库（git@协议）；
2. 或者，如果你坚持尝试 Level1 最小补丁方案，给出需要修改的源码文件位置 + 补丁片段。

你倾向哪条路线？