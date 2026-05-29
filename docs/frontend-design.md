# 🎭 LaLaLand 前端设计方案 (v3 - 深蓝紫色调)

> **参考基准**：[salesmanbroadway.com](https://salesmanbroadway.com/) · [lesmis.com](https://lesmis.com/)
> **核心原则**：叙事驱动，不是模板拼接。像设计一张剧场海报，而不是一个网站。

---

## 1. 设计哲学

1. **Full-bleed imagery**——全出血图片，不做小卡片网格
2. **Cinematic typography**——字号落差至少 4 倍，Serif + Sans-serif 经典搭配
3. **Narrative scroll**——滚动 = 翻阅一个故事
4. **Restraint in motion**——只有 3 种动效：fade-up / image reveal / hover glow

---

## 2. 🎨 色彩系统 (深蓝 + 紫色主调)

```css
:root {
  --bg:       #0B1026;     /* 深蓝夜空——主背景 */
  --surface:  #111833;     /* 次级面：导航、卡片 */
  --purple:   #7C3AED;     /* 紫色强调——CTA、hover、活跃态 */
  --cream:    #E8E4F0;     /* 主文字：带紫调的浅灰白 */
  --muted:    #5E6080;     /* 次要文字、标注 */
}
```

深蓝 `#0B1026` 是 La La Land 天文台夜景的基调色。紫色 `#7C3AED` 是霓虹灯的点缀。只需要这两个色，加上乳白文字和灰色标注，就够了。

### 渐变（仅 2 个）

```css
--grad-hero:  linear-gradient(180deg, #0B1026 0%, #1A1145 50%, #0B1026 100%);
/* 用于 Hero section——模拟夜空中心微亮的紫色穹顶 */

--grad-accent: linear-gradient(135deg, #5B21B6, #7C3AED, #A78BFA);
/* 用于 CTA 按钮 hover 态——紫色三阶渐变 */
```

---

## 3. 排版系统

| 层级 | 字体 | 移动端 | 桌面端 | 字重 |
|------|------|--------|--------|------|
| Display | Playfair Display | 48px | 120px | 300 light |
| H1 | Playfair Display | 32px | 56px | 400 |
| H2 | Inter | 20px | 28px | 600 |
| Body | Inter | 16px | 18px | 400 |
| Caption | Inter | 12px | 14px | 500, uppercase, `letter-spacing: 0.15em` |

---

## 4. 动效系统 (只有 3 种)

| 效果 | 用在哪 | 参数 |
|------|--------|------|
| Fade-up on scroll | section 内容进入视口 | `opacity 0→1, translateY 24→0, 600ms ease-out` |
| Image reveal | 剧照渐现 | `clip-path inset(100% 0 0 0) → inset(0), 800ms` |
| Hover glow | CTA 按钮、链接 | `color → purple, 200ms` |

---

## 5. 页面结构

### Hero（首屏）

```
╔══════════════════════════════════════════════════╗
║                                                  ║
║    深蓝夜空渐变背景 (#0B1026 → #1A1145 → #0B1026) ║
║    [占位：全屏剧照暗化叠底]                        ║
║                                                  ║
║     L  A     L  A     L  A  N  D                ║  ← Display, 120px, light
║                                                  ║     文字色 --cream
║       爱 乐 之 城  ·  毕 业 话 剧                ║  ← H2, purple, letter-spacing
║                                                  ║
║            ● 预 约 座 位                          ║  ← 按钮：purple outline,
║                                                  ║     hover → 紫色渐变填充
║        ────── ↓ 探索更多 ──────                  ║
║                                                  ║
╚══════════════════════════════════════════════════╝
```

### 其他 section

沿用叙事结构：
- **演出信息 + 倒计时**（大数字，紫色分隔线）
- **故事**（半屏占位剧照 + 文字）
- **演员表**（占位名单）
- **画廊**（占位格子）
- **Footer**

所有 section 标题用 `--muted` 色的 uppercase caption，内容用 `--cream`，强调元素用 `--purple`。

---

## 6. 选座图配色

```
背景：  #0B1026（与全站一致）
舞台：  紫色渐变光幕 (--grad-accent 半透明叠底)
空闲：  rgba(232, 228, 240, 0.25)（cream 25%）
我的：  #7C3AED 实心 + 微 glow
他人：  #1E2040（比 surface 稍亮的深色）
锁定：  #1A1A2E + 删除线图标
```

---

## 7. 技术栈

* Vue 3 + Vite 5 
* **Vanilla CSS** (放弃 TailwindCSS，精细控制排版)
* Pinia 
* vue-router 4 
* axios 
* 字体: Inter + Playfair Display

---

## 附录: VibeHub CLI Bug 诊断记录

> 记录于 2026-05-29

**症状：**
`vibehub task create` 或 `vibehub --help` 等命令在当前 macOS 设备上 100% 触发挂起，无法在 CLI 中正常使用。

**诊断分析：**
1. `vibehub` 是 Mach-O arm64 二进制文件。
2. 该二进制链接了 **AppKit.framework**（macOS GUI 框架）。
3. 在终端纯 CLI 调用时，AppKit 的初始化会触发 IMK (Input Method Kit) 输入法框架的 mach port 错误（`error messaging the mach port for IMKCFRunLoopWakeUpReliable`），导致进程永久挂起。
4. `--help` / `task create` / `task list` 等子命令全部受影响。唯独 `vibehub sync` 能成功返回（可能走了不触发 AppKit 初始化的不同代码路径）。

**已尝试但无效的绕过方法：**
- `VIBEHUB_NO_TTY=1`
- `QT_QPA_PLATFORM=offscreen DISPLAY=`
- `& sleep N; kill` 超时 kill

**当前应对策略：**
由于无法通过 CLI 自动注册新 task 到 VibeHub 的 active 列表中，前端开发可直接在**已有 task** `T-20260529133755-69e7bd66`（标题 `lalaland-platform`）下继续，或者在 VibeHub **GUI 界面中手动创建**。
