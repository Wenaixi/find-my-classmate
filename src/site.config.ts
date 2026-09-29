/**
 * 站点信息配置
 *
 * 修改此文件后重新构建即可，无需改动任何业务代码。
 *
 * 三个字段都是必填字符串：类型上无 undefined，因此「留空」只能是空串，
 * 而 App.tsx 对 dataSource 与 team 无条件渲染，空串会渲染出空壳文案。
 * dataController 虽带真值判断，但 as const 下恒为真——该分支是历史遗留。
 */
const siteConfig = {
  /** 页脚：数据来源说明 */
  dataSource: "福清一中公示数据提取",

  /** 页脚：运营团队名称 */
  team: "福清一中信息社",

  /** 页脚：隐私说明中的数据处理方 */
  dataController: "福清一中信息社",
} as const;

export default siteConfig;
