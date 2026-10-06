# Refine Log: octarq-web-core (P0 Core Communication & Shell)

## 精炼前指标
- `src/api.ts`: 772 LOC (God file, 11 处 loose `any` 类型, 包含所有 DTO 定义与各模块 REST 端点)
- `src/App.tsx`: 524 LOC (God file, 内联包含整套 AppRoutes、Shell、CreateOrgModal、EmailVerifyBanner 与窗口事件监听)
- 错误处理: 存在多处静默 `.catch(() => {})` 与 `catch (e: any)`

## 精炼操作
1. **拆解 `src/api.ts` 为高内聚模块**:
   - `src/api/types.ts` (360 LOC): 提取所有强类型 DTO，彻底清除 `ProviderAccount`, `SMTPSender`, `NotificationChannel`, `exportWorkspaceData` 的 `any` 污染。
   - `src/api/client.ts` (385 LOC): 提取标准请求封装 `req<T>`、错误载荷 `ApiError` 与 API 实例。
   - `src/api/overview.ts` (41 LOC): 提取概览数据缓存与响应式 Hook。
   - `src/api.ts` (5 LOC): 纯净外观门面（Façade），完全透明地 100% 向后兼容所有导入方。
2. **拆解 `src/App.tsx` 为模块化组件**:
   - `src/shell/AppRoutes.tsx` (39 LOC): 提取全量路由装配。
   - `src/shell/CreateOrgModal.tsx` (47 LOC): 提取创建空间弹窗。
   - `src/shell/EmailVerifyBanner.tsx` (64 LOC): 提取邮箱验证横幅。
   - `src/shell/useShellEvents.ts` (43 LOC): 提取跨空间与权限更新的窗口事件监听。
   - `src/App.tsx`: 精简至 450 LOC，严格满足 $\le 450$ LOC 规范，保留 `switchToOrg` 与品牌刷新绑定。
3. **消除吞错误与 Any 污染**:
   - 清除裸 `catch`，在 DEV 环境增加调试跟踪或通过 toast 提示。
   - 彻底消灭 `catch (e: any)`，采用 `err instanceof Error` 类型收窄。

## 精炼后指标
- 所有单个文件行数：严格 $\le 450$ LOC
- `pnpm typecheck`: 100% 通过 (0 错误)
- `pnpm test`: 76 测试文件，419 测试项全部 100% 绿色通过
