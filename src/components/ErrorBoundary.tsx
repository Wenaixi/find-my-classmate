import { Component, type ReactNode } from "react";

interface Props {
  children: ReactNode;
}

interface State {
  hasError: boolean;
}

// F1：异步 chunk 加载失败（网络抖动/部署切换后旧 chunk 404）时，
// 错误会冒泡到 React 根导致整页白屏。此边界捕获错误并给出可恢复的 UI。
export default class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false };

  static getDerivedStateFromError(): State {
    return { hasError: true };
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className="result-message" role="alert">
          <strong>组件加载失败</strong>
          <p>页面部分功能未能加载，请刷新页面重试。</p>
          <button
            className="text-action"
            type="button"
            onClick={() => {
              this.setState({ hasError: false });
              window.location.reload();
            }}
          >
            刷新页面 <span aria-hidden="true">↗</span>
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}
