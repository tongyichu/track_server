import { useLayoutEffect } from "react";
import { createRoot } from "react-dom/client";
import { Alert } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import "./workbench.css";

const growth = [
  { id: "activeUsers", label: "登录用户", read: "users" },
  { id: "newUsers", label: "新增用户", read: "users" },
  { id: "uploadedTracks", label: "新增云端轨迹", read: "tracks" },
  { id: "uploadUsers", label: "上传用户", read: "tracks" },
  { id: "submissions", label: "新增投稿", read: "track_submissions" },
];

const quality = [
  { id: "avgDistance", label: "平均距离" },
  { id: "avgDuration", label: "平均时长" },
  { id: "staleRunning", label: "运行超过 24 小时", tone: "warning" },
  { id: "zeroDistance", label: "零距离轨迹", tone: "warning" },
  { id: "failedMapJobs", label: "索引失败", read: "route_groups", tone: "danger" },
  { id: "pendingMapJobs", label: "待建索引", read: "route_groups" },
  { id: "activeRouteGroups", label: "有效路线组", read: "route_groups" },
];

const engagement = [
  { id: "engagementCollections", label: "收藏" },
  { id: "engagementNavigations", label: "导航" },
  { id: "engagementLikes", label: "点赞" },
  { id: "engagementComments", label: "评论" },
];

function Section({ id, title, read, hidden, action, children }) {
  const headingId = id ? id + "Heading" : undefined;
  return (
    <section id={id} aria-labelledby={headingId} className="workbench-section" data-admin-read={read} hidden={hidden}>
      <div className="workbench-section-heading">
        <h2 id={headingId}>{title}</h2>
        {action}
      </div>
      {children}
    </section>
  );
}

function Panel({ title, detail, read, children }) {
  return (
    <Card data-admin-read={read} className="workbench-panel min-w-0 gap-0 rounded-xl border-border py-0 shadow-none">
      <CardHeader className="workbench-panel-head flex items-center justify-between gap-2 px-4 py-3">
        <CardTitle><h3>{title}</h3></CardTitle>
        {detail}
      </CardHeader>
      <CardContent className="min-w-0 px-4 pb-3 pt-1">{children}</CardContent>
    </Card>
  );
}

function Metric({ id, label, read, href, tone, comparison }) {
  const content = (
    <>
      <span>{label}</span>
      <strong id={id}>—</strong>
      {comparison && <small id={id + "Delta"} />}
    </>
  );
  if (href) {
    return <a className="workbench-metric workbench-metric-link" data-admin-read={read} href={href}>{content}</a>;
  }
  return <div className="workbench-metric" data-admin-read={read} data-tone={tone}>{content}</div>;
}

function Metrics({ items, className = "", comparison = false }) {
  return (
    <div className={"workbench-metrics " + className}>
      {items.map((item) => <Metric key={item.id} {...item} comparison={comparison} />)}
    </div>
  );
}

function Chart({ id, label, size = "medium" }) {
  return <div id={id} className={"dashboard-chart chart-" + size} role="img" aria-label={label} />;
}

function Ring({ id, label, size }) {
  return (
    <div className="dashboard-ring">
      <Chart id={id} label={label} size={size} />
      <div id={id + "Legend"} className="admin-user-legend" />
    </div>
  );
}

function Fact({ id, label }) {
  return <div className="workbench-fact"><span>{label}</span><strong id={id}>—</strong></div>;
}

function Header() {
  return (
    <div className="workbench-header">
      <div>
        <div className="workbench-title">
          <h1>运营工作台</h1>
          <span id="analyticsHealth" data-admin-read="analytics" aria-live="polite" hidden>
            <Badge asChild variant="outline" className="workbench-health rounded-full border-border px-2.5 py-1">
              <a href="/admin/analytics.html" id="analyticsHealthLink" data-state="pending" title="仅检查埋点归档链路">
                <span className="health-dot" aria-hidden="true" />
                <span id="analyticsHealthLabel">埋点 · 检查中</span>
              </a>
            </Badge>
          </span>
        </div>
        <div className="workbench-time"><span id="rangeHint" /><span id="updatedAt" /></div>
      </div>
      <div className="workbench-actions">
        <div className="workbench-periods" role="group" aria-label="统计周期">
          {[["day", "日"], ["week", "周"], ["month", "月"]].map(([period, label]) => (
            <Button key={period} type="button" variant="ghost" size="sm" data-period={period}
              aria-pressed={false} className="h-8 min-w-10 rounded-md">{label}</Button>
          ))}
        </div>
        <Button type="button" variant="outline" size="sm" id="refreshDashboard" className="h-9 rounded-md">刷新数据</Button>
      </div>
    </div>
  );
}

function ManagementTable({ id, title, read, note, columns, link, period = false }) {
  return (
    <Section id={id + "Section"} title={title} read={read}
      action={link && <a className="workbench-link" href={link[0]}>{link[1]}</a>}>
      <Card data-workbench-table={id} data-workbench-period={period ? "" : undefined}
        className="workbench-panel min-w-0 gap-0 rounded-xl border-border py-0 shadow-none">
        <CardContent className="min-w-0 px-4 py-3">
          <p className="workbench-table-note">{note} <span data-table-status aria-live="polite" /></p>
          <Table className="workbench-table">
            <TableHeader><TableRow>{columns.map((label) => <TableHead key={label} scope="col">{label}</TableHead>)}</TableRow></TableHeader>
            <TableBody><TableRow><td colSpan={columns.length}>正在读取数据…</td></TableRow></TableBody>
          </Table>
        </CardContent>
      </Card>
    </Section>
  );
}

function Workbench() {
  useLayoutEffect(() => {
    window.AdminAccess.apply();
    root.dispatchEvent(new Event("admin:dashboard-ready"));
  }, []);

  return (
    <>
      <Header />
      <p id="dashboardStatus" className="workbench-time" role="status" hidden />
      <Alert variant="destructive" id="dashboardError" className="hidden workbench-alert block" />
      <Section id="dashboardGrowth" title="本期增长" hidden>
        <Metrics items={growth} comparison />
      </Section>
      <Alert variant="destructive" id="dashboardChartError" className="hidden workbench-alert block" />
      <ManagementTable id="cities" title="城市路线供需" read="tracks" period
        note="按路线所在城市统计，依据本期导航次数排列。供给是当前公开、已完成且有原始文件的路线。"
        columns={["城市", "路线存量", "本期新增", "新增收藏人数", "导航上报", "较上期"]} />
      <ManagementTable id="cohorts" title="新用户留存与首条轨迹" read="users tracks"
        note="近 6 个注册周。回访以成功登录为准，观察期结束后显示比例。"
        columns={["注册周", "注册人数", "7 天内首条云端轨迹", "次日登录回访", "第 7 日登录回访"]} />
      <Section title="增长与转化">
        <div className="workbench-grid workbench-grid-main">
          <Panel title="用户与轨迹趋势" read="users tracks" detail={<span id="trendRangeHint" className="workbench-time" />}>
            <Chart id="trendChart" size="large" label="用户和轨迹趋势图" />
          </Panel>
          <Panel title="新增用户转化" read="users tracks track_submissions">
            <Chart id="funnelChart" size="large" label="本周期注册用户的上传、投稿及审核转化" />
          </Panel>
        </div>
      </Section>
      <Section title="内容供给与质量" read="tracks"
        action={<a className="workbench-link" href="/admin/tracks.html">管理轨迹</a>}>
        <Metrics items={quality} className="workbench-quality" />
        <div className="workbench-grid workbench-grid-equal">
          <Panel title="公开状态"><Ring id="trackStatusChart" label="轨迹公开状态" /></Panel>
          <Panel title="运动类型"><Ring id="trackTypeChart" label="运动类型" /></Panel>
        </div>
      </Section>
      <ManagementTable id="versions" title="安卓版本表现" read="releases" link={["/admin/index.html", "查看发布管理"]}
        note="近 7 个北京时间自然日，活跃设备按最近上报版本归属，按版本名合并构建。"
        columns={["版本", "发布记录日期", "活跃设备", "设备占比", "反馈"]} />
      <Section title="互动与同行">
        <div className="workbench-grid workbench-grid-main">
          <Panel title="本期内容互动" read="tracks">
            <Metrics items={engagement} className="workbench-engagement" comparison />
            <span id="engagementRangeHint" className="workbench-time" />
            <Chart id="engagementChart" size="large" label="每日内容互动趋势" />
          </Panel>
          <Panel title="同行概览" read="companions"
            detail={<a className="workbench-link" href="/admin/companions.html">查看会话</a>}>
            <div className="workbench-companion-primary">
              <span>当前活跃</span><strong id="companionActive">—</strong><small>个同行会话</small>
            </div>
            <Fact id="companionCreated" label="本周期新增" />
            <Fact id="companionEnded" label="本周期结束" />
            <Fact id="companionAutoEnded" label="自动结束" />
            <Fact id="companionParticipants" label="新增房间平均参与人数" />
          </Panel>
        </div>
      </Section>
      <ManagementTable id="companions" title="与友同行完成情况" read="companions tracks" period
        link={["/admin/companions.html", "查看同行房间"]}
        note="按房间结束时间归期。同一用户参加不同房间分别计入人次。"
        columns={["运动类型", "已结束房间", "成员人次", "轨迹已生成", "待同步", "无有效轨迹点", "轨迹不可用"]} />
      <Section title="本期路线使用排行" read="tracks">
        <Panel title="按导航上报次数排序" detail={<span className="workbench-time">并列时按收藏、点赞、评论排列</span>}>
          <Table className="workbench-table">
            <TableHeader><TableRow>{["轨迹", "导航上报", "收藏", "点赞", "评论"].map((label) => <TableHead key={label} scope="col">{label}</TableHead>)}</TableRow></TableHeader>
            <TableBody id="topTracks" />
          </Table>
        </Panel>
      </Section>
      <Section id="dashboardEfficiency" title="本期运营效率" hidden>
        <div className="workbench-grid workbench-grid-equal">
          <Panel title="投稿审核" read="track_submissions">
            <Fact id="approvalRate" label="审核通过率" />
            <Fact id="avgReviewHours" label="平均审核耗时" />
            <details id="reviewBreakdown" className="workbench-disclosure" hidden>
              <summary>审核结果分布</summary>
              <Ring id="reviewChart" size="small" label="审核结果分布" />
            </details>
          </Panel>
          <Panel title="意见反馈" read="feedbacks">
            <Fact id="feedbackResolvedPeriod" label="已解决反馈" />
            <Fact id="avgResolutionHours" label="平均解决耗时" />
            <details id="feedbackBreakdown" className="workbench-disclosure" hidden>
              <summary>当前状态与本期反馈平台</summary>
              <div className="workbench-grid workbench-grid-equal">
                <Ring id="feedbackStatusChart" size="small" label="当前反馈状态分布" />
                <Chart id="feedbackVersionChart" size="small" label="本期反馈平台分布" />
              </div>
            </details>
          </Panel>
        </div>
      </Section>
      <ManagementTable id="backlog" title="待办与积压"
        note="当前快照。超过提交或可执行时间 24 小时计为积压。注销文件等待凭证到期期间不计积压。"
        columns={["事项", "未结数", "积压数", "最长等待", "处理入口"]} />
    </>
  );
}

const root = document.getElementById("dashboard-workbench");
createRoot(root).render(<Workbench />);
