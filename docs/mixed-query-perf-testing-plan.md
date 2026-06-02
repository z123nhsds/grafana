# 多数据源混合查询面板：端到端性能测试方案 (百万级数据流式合并与虚拟滚动)

## 背景

为了满足多数据源聚合监控分析的需求，新的“多数据源混合查询”面板需要支持百万级时间点的数据聚合、流式接收，并在前端保证渲染时不卡顿。为保障产品质量与性能，我们需要构建端到端（E2E）测试体系，覆盖后端数据处理、前端状态管理以及 UI 虚拟滚动渲染。

## 测试方案设计

本方案在不引入新依赖的前提下，复用了 Grafana 的原生测试生态：
- 后端：使用 `testing` 和 `grafana-plugin-sdk-go/backend`。
- 前端组件：使用 `Jest` 和 `@testing-library/react` 以及 `@grafana/data` 内部 Mock。
- E2E 测试：使用 `@grafana/plugin-e2e`（Playwright 封装）。

### 1. 后端集成测试 (Go)
**目标**：验证在 `QueryData` 和 `CallResource` 中进行百万级时间点多数据源对齐与合并时的性能。
- **文件路径**: `pkg/tsdb/mixedquery/mixed_query_test.go`
- **内容**：
  - Mock 了 `QueryData` 请求，生成 1,000,000 个数据点（包含多个 `Field` 以模拟多数据源的时间对齐合并）。
  - 测试断言：执行时间应小于 500ms。
  - Mock 了 `CallResource` 请求，模拟流式数据（Streaming）的传输，切片返回百万级数据点，验证通信与序列化开销。

### 2. 前端单元测试 (Jest + Testing Library)
**目标**：验证 React 组件能够高效响应 RxJS 数据流推送（Data Query Response stream）并正确更新 Redux 状态，不导致线程阻塞。
- **文件路径**: `public/app/plugins/panel/mixedquery/MixedQueryPanel.test.tsx`
- **内容**：
  - 模拟了一个带有虚拟滚动（Virtual Scroll）视口的 React 组件。
  - 使用 `@reduxjs/toolkit` 创建独立的 Mock Store 验证状态分发。
  - 通过 `act` 配合分块数据推送，模拟 RxJS 推送的多次大体积数据。
  - 测试断言：
    - 面板状态由 `Streaming` 正确转换为 `Done`。
    - Redux Store 中的数据量最终记录为 1,000,000。
    - DOM 中渲染的行数被严格限制在可视区域大小（< 100 行），证明虚拟滚动生效且 DOM 树未被撑爆。

### 3. E2E 测试 (Playwright)
**目标**：真实浏览器环境验证 UI 响应度与 DOM 节点数量，确保“虚拟滚动只渲染可视行”。
- **文件路径**: `e2e-playwright/panels-suite/mixedquery.spec.ts`
- **内容**：
  - 使用 `@grafana/plugin-e2e` 拦截（Mock）Query API 响应，注入百万级模拟数据。
  - 测试断言：
    - 加载百万数据后，通过 Locator 统计 `[data-testid="visible-row"]`，确保节点数远小于实际数据量（例如小于 100）。
    - 触发滚动条事件后（`scrollTop = 50000`），DOM 仍能保持在较小量级。
    - 通过 `requestAnimationFrame` 检测 UI 线程不卡顿（每帧渲染耗时 < 50ms）。

## 如何运行测试

```bash
# 1. 运行后端集成测试
go test -run TestMixedQuery_ -v ./pkg/tsdb/mixedquery/

# 2. 运行前端 Jest 测试
yarn test public/app/plugins/panel/mixedquery/MixedQueryPanel.test.tsx

# 3. 运行 E2E 测试
yarn e2e:playwright e2e-playwright/panels-suite/mixedquery.spec.ts
```

## 代码生成清单

- `pkg/tsdb/mixedquery/mixed_query_test.go`
- `public/app/plugins/panel/mixedquery/MixedQueryPanel.test.tsx`
- `e2e-playwright/panels-suite/mixedquery.spec.ts`
- `docs/mixed-query-perf-testing-plan.md` (本文档)
