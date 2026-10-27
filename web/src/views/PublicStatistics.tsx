/* Constraint brief: primary = real sample counts and project/drop statistics;
   secondary = filters and half-hour comparisons; tertiary = methodology/API links.
   One primary action: apply filters. Reuse existing light CSS tokens: --accent
   #056dff, --text #18181b, --panel, --line, 8px radius, 8/16/24/32px spacing.
   No chart dependency; keyboard-accessible bars keep raw denominators inspectable. */
import { useEffect, useMemo, useState } from 'react'
import { Logo } from '../ui'
import './PublicStatistics.css'

interface Group {
  series: number; project: string; scene: string; appearances: number; samples: number
  duration_seconds: number; appearance_rate: number
}
interface Item extends Group {
  item: string; tag: string; occurrences: number; amount: number; timed_samples: number
  average_per_sample: number; drop_rate: number; amount_per_hour: number | null
}
interface Summary {
  from: string; to: string; groups: Group[]; items: Item[]
  daily: { day: string; samples: number; appearances: number }[]
}
interface Slot {
  slot_index: number; slot_start: string; series: number; project: string; scene: string
  appearances: number; samples: number; total_appearances: number; appearance_rate: number
}
interface Filters { days: string; server: string; series: string; focus: string; scene: string }
const defaults: Filters = { days: '30', server: 'cn', series: '', focus: '', scene: '' }
const servers = [['', '全部区服'], ['cn', '国服安卓'], ['cn_ios', '国服 iOS'], ['cn_channel', '国服渠道'], ['en', '国际服'], ['jp', '日服'], ['tw', '台服']]
const tags: Record<string, string> = { '': '普通', bonus: '额外', event: '活动', catchup: '追赶', meow: '猫箱', log: '记录', scan: '扫描' }
const number = (n: number) => n.toLocaleString('zh-CN')
const percent = (n: number) => `${(n * 100).toFixed(2)}%`
const groupLabel = (g: Pick<Group, 'series' | 'project'>) => g.series ? `第 ${g.series} 期 · ${g.project}` : g.project || '通用掉落'
const keyOf = (g: Pick<Group, 'series' | 'project'>) => `${g.series}:${g.project}`

// Public requests never read admin localStorage or attach an Authorization header.
async function readPublic<T>(path: string, signal: AbortSignal): Promise<T> {
  const response = await fetch(path, { signal, credentials: 'omit' })
  const body = await response.json()
  if (!response.ok) throw new Error(body.error?.message || `查询失败（${response.status}）`)
  return body as T
}

function TimeSlots({ slots, groups }: { slots: Slot[]; groups: Group[] }) {
  const options = useMemo(() => [...new Map(groups.filter((g) => g.appearances > 0).map((g) => [keyOf(g), g])).values()], [groups])
  const [choice, setChoice] = useState('')
  const selected = options.find((g) => keyOf(g) === choice) ?? options[0]
  const [active, setActive] = useState<number | null>(null)
  const bins = Array.from({ length: 48 }, (_, index) => {
    const rows = slots.filter((s) => s.slot_index === index)
    const total = Math.max(0, ...rows.map((s) => s.total_appearances))
    const count = rows.filter((s) => selected && keyOf(s) === keyOf(selected)).reduce((sum, s) => sum + s.appearances, 0)
    return { index, total, count, rate: total ? count / total : 0, label: `${String(Math.floor(index / 2)).padStart(2, '0')}:${index % 2 ? '30' : '00'}` }
  })
  const scale = Math.max(0.05, ...bins.map((b) => b.rate))
  const highlighted = active === null ? null : bins[active]
  return <section className="public-card slot-card">
    <div className="public-section-head"><div><p className="public-eyebrow">TIME DISTRIBUTION</p><h2>半小时出现占比</h2></div><label>对比项目<select value={selected ? keyOf(selected) : ''} onChange={(e) => setChoice(e.target.value)} disabled={!options.length}>
      {!options.length && <option value="">暂无项目观测</option>}{options.map((g) => <option key={keyOf(g)} value={keyOf(g)}>{groupLabel(g)}</option>)}
    </select></label></div>
    <p className="public-note">北京时间（UTC+8），按截图采集时刻合并各日同一时段。分母是该时段全部已识别项目槽位数。</p>
    <div className="slot-readout" aria-live="polite">{highlighted ? <><strong>{highlighted.label}</strong><span>{highlighted.total ? `${percent(highlighted.rate)} · ${number(highlighted.count)} / ${number(highlighted.total)} 个项目槽位` : '尚无观测，不代表出现率为零'}</span></> : <><strong>{selected ? groupLabel(selected) : '等待有效观测'}</strong><span>指向或聚焦柱条查看样本量</span></>}</div>
    <div className="slot-scale"><span>{percent(scale)}</span><span>0%</span></div>
    <div className="slot-scroll"><div className="slot-bars" aria-label="48 个半小时时段的项目出现占比">
      {bins.map((bin) => <button type="button" key={bin.index} className={`slot-column ${bin.total ? '' : 'slot-missing'}`} onMouseEnter={() => setActive(bin.index)} onFocus={() => setActive(bin.index)} onClick={() => setActive(bin.index)} aria-label={`${bin.label}，${bin.total ? `${percent(bin.rate)}，${bin.count}/${bin.total} 个项目槽位` : '无观测'}`}>
        <span className="slot-bar" style={{ height: `${bin.total ? Math.max(1, bin.rate / scale * 100) : 1}%` }} />
        {bin.index % 8 === 0 && <span className="slot-label">{bin.label}</span>}
      </button>)}
    </div></div>
    {!options.length && <p className="public-note">当前筛选没有科研列表观测，可尝试切换区服、期数或倾向。</p>}
    <p className="public-footnote">空白时段表示缺少观测。样本较少时占比波动较大，请结合分母判断。</p>
  </section>
}

function ItemsTable({ items }: { items: Item[] }) {
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(0)
  const filtered = items.filter((row) => `${row.item} ${row.project} ${row.scene}`.toLowerCase().includes(search.toLowerCase()))
  const pages = Math.max(1, Math.ceil(filtered.length / 25))
  const current = Math.min(page, pages - 1)
  return <section className="public-card">
    <div className="public-section-head"><div><p className="public-eyebrow">REWARDS</p><h2>掉落明细</h2></div><label>搜索物品 / 项目 / 场景<input type="search" value={search} onChange={(e) => { setSearch(e.target.value); setPage(0) }} placeholder="例如 Blueprint、Coins" /></label></div>
    <p className="public-note">每行按项目、场景和掉落标签独立统计；单次均量包含未掉落该物品的样本。</p>
    {filtered.length ? <><div className="table-wrap"><table className="public-table"><thead><tr><th>物品</th><th>项目 / 场景</th><th>总数量</th><th>掉落样本 / 有效样本</th><th>掉落率</th><th>单次均量</th><th>每小时</th></tr></thead><tbody>
      {filtered.slice(current * 25, (current + 1) * 25).map((row) => <tr key={`${keyOf(row)}:${row.scene}:${row.item}:${row.tag}`}><td><strong>{row.item}</strong><span className="public-cell-note">{tags[row.tag] ?? row.tag}</span></td><td>{groupLabel(row)}<span className="public-cell-note">{row.scene || '未标注场景'}</span></td><td>{number(row.amount)}</td><td>{number(row.occurrences)} / {number(row.samples)}</td><td>{percent(row.drop_rate)}</td><td>{row.average_per_sample.toFixed(3)}</td><td>{row.amount_per_hour === null ? '—' : row.amount_per_hour.toFixed(3)}</td></tr>)}
    </tbody></table></div><div className="public-pagination"><span>{number(filtered.length)} 条 · 第 {current + 1} / {pages} 页</span><div><button className="ghost" disabled={current === 0} onClick={() => setPage(current - 1)}>上一页</button><button className="ghost" disabled={current + 1 >= pages} onClick={() => setPage(current + 1)}>下一页</button></div></div></> : <div className="public-empty"><strong>{search ? '没有符合搜索的物品' : '当前筛选暂无掉落数据'}</strong><p>仅识别完整的结果计入统计。识别失败的截图会在模板补齐并重识别后更新。</p></div>}
  </section>
}

export function PublicStatistics() {
  const [view, setView] = useState<'research' | 'drop'>(() => new URLSearchParams(location.search).get('view') === 'drop' ? 'drop' : 'research')
  const [draft, setDraft] = useState<Filters>(defaults)
  const [applied, setApplied] = useState<Filters>(defaults)
  const [tick, setTick] = useState(0)
  const [data, setData] = useState<{ summary: Summary; slots: Slot[] } | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [query, setQuery] = useState('')
  useEffect(() => { document.title = 'AzurPilot 统计 · 科研与掉落' }, [])
  useEffect(() => {
    const controller = new AbortController()
    setLoading(true); setError(''); setData(null)
    const to = new Date()
    const params = new URLSearchParams({ from: new Date(to.getTime() - Number(applied.days) * 86400000).toISOString(), to: to.toISOString(), timezone: 'Asia/Shanghai' })
    if (applied.server) params.set('server', applied.server)
    if (applied.scene.trim()) params.set('scene', applied.scene.trim())
    if (view === 'research' && applied.series) params.set('series', applied.series)
    if (view === 'research' && applied.focus) params.set('focus_series', applied.focus)
    const path = `/api/v2/get/${view}?${params}`
    setQuery(path)
    Promise.all([
      readPublic<Summary>(path, controller.signal),
      view === 'research' ? readPublic<{ slots: Slot[] }>(`/api/v2/get/research/time-slots?${params}`, controller.signal) : Promise.resolve({ slots: [] }),
    ]).then(([summary, times]) => { if (!controller.signal.aborted) setData({ summary, slots: times.slots }) })
      .catch((e: Error) => { if (!controller.signal.aborted) setError(e.message) })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [view, applied, tick])
  const summary = data?.summary
  const samples = summary?.daily.reduce((sum, d) => sum + d.samples, 0) ?? 0
  const appearances = summary?.daily.reduce((sum, d) => sum + d.appearances, 0) ?? 0
  const itemCount = new Set(summary?.items.map((i) => i.item)).size
  const totalAmount = summary?.items.reduce((sum, i) => sum + i.amount, 0) ?? 0
  const switchView = (next: 'research' | 'drop') => { setView(next); history.replaceState(null, '', next === 'drop' ? '/?view=drop' : '/') }
  return <div className="public-site">
    <header className="public-topbar"><a className="public-brand" href="/"><Logo size={32} /><strong>AzurPilot <span>统计</span></strong></a><nav aria-label="网站导航"><a href="/admin/">管理后台 ↗</a></nav></header>
    <main className="public-main">
      <div className="public-intro"><div><p className="public-eyebrow">COMMUNITY STATISTICS</p><h1>让每一份记录，都有参考价值。</h1><p>汇总社区上传的科研与掉落截图，观察项目分布和时段差异。</p></div><span className="public-badge">公开查询 · 无需登录</span></div>
      <nav className="public-tabs" aria-label="统计类型"><button className={view === 'research' ? 'active' : ''} aria-current={view === 'research' ? 'page' : undefined} onClick={() => switchView('research')}>科研统计</button><button className={view === 'drop' ? 'active' : ''} aria-current={view === 'drop' ? 'page' : undefined} onClick={() => switchView('drop')}>通用掉落</button></nav>
      <form className="public-filters" onSubmit={(e) => { e.preventDefault(); setApplied({ ...draft }); setTick((n) => n + 1) }}>
        <label>统计区间<select value={draft.days} onChange={(e) => setDraft({ ...draft, days: e.target.value })}>{[1, 7, 30, 90, 365].map((d) => <option key={d} value={d}>最近 {d} 天</option>)}</select></label>
        <label>区服<select value={draft.server} onChange={(e) => setDraft({ ...draft, server: e.target.value })}>{servers.map(([id, title]) => <option key={id} value={id}>{title}</option>)}</select></label>
        {view === 'research' && <><label>项目期数<select value={draft.series} onChange={(e) => setDraft({ ...draft, series: e.target.value })}><option value="">全部期数</option>{Array.from({ length: 10 }, (_, n) => <option key={n + 1} value={n + 1}>第 {n + 1} 期</option>)}</select></label><label>科研倾向<select value={draft.focus} onChange={(e) => setDraft({ ...draft, focus: e.target.value })}><option value="">全部倾向（含未识别）</option>{Array.from({ length: 10 }, (_, n) => <option key={n + 1} value={n + 1}>倾向第 {n + 1} 期</option>)}</select></label></>}
        <label className="public-scene">场景<input value={draft.scene} onChange={(e) => setDraft({ ...draft, scene: e.target.value })} placeholder="全部场景" maxLength={128} /></label>
        <button type="submit" disabled={loading}>{loading ? '查询中…' : '应用筛选'}</button>
      </form>
      <div role="status" className="public-query-status">{loading ? '正在读取最新统计…' : summary ? `数据截至 ${new Date(summary.to).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false })}（北京时间）` : '查询未完成'}<span>每 30 分钟派发新截图；重识别结果回传后更新</span></div>
      {error && <div className="public-error" role="alert"><strong>暂时无法取得统计</strong><p>{error}</p><button className="ghost" onClick={() => setTick((n) => n + 1)}>重试查询</button></div>}
      {loading && <div className="public-loading" aria-label="统计加载中"><span /><span /><span /></div>}
      {summary && <>
        <div className="public-metrics">{[
          [view === 'research' ? '已观测项目槽位' : '有效掉落样本', view === 'research' ? appearances : samples, view === 'research' ? '每张完整科研列表通常含 5 个项目' : '按识别后的掉落事件计数'],
          [view === 'research' ? '科研完成记录' : '物品种类', view === 'research' ? samples : itemCount, view === 'research' ? '仅含项目和奖励均识别完整的记录' : '按物品标识去重'],
          [view === 'research' ? '掉落物品种类' : '累计掉落数量', view === 'research' ? itemCount : totalAmount, '当前筛选范围内的有效结果'],
        ].map(([title, value, note]) => <section key={title} className="public-metric"><p>{title}</p><strong>{number(Number(value))}</strong><span>{note}</span></section>)}</div>
        {view === 'research' && <><TimeSlots slots={data?.slots ?? []} groups={summary.groups} /><section className="public-card"><div className="public-section-head"><div><p className="public-eyebrow">PROJECTS</p><h2>科研项目分布</h2></div><span className="public-note">{summary.groups.length} 条项目 / 场景记录</span></div>
          {summary.groups.length ? <div className="table-wrap"><table className="public-table"><thead><tr><th>项目</th><th>场景</th><th>出现次数</th><th>出现占比</th><th>完成记录</th><th>记录总时长</th></tr></thead><tbody>{summary.groups.map((g) => <tr key={`${keyOf(g)}:${g.scene}`}><td><strong>{groupLabel(g)}</strong></td><td>{g.scene}</td><td>{number(g.appearances)}</td><td><span className="public-rate"><i style={{ width: `${g.appearance_rate * 100}%` }} /></span>{percent(g.appearance_rate)}</td><td>{number(g.samples)}</td><td>{(g.duration_seconds / 3600).toFixed(1)} h</td></tr>)}</tbody></table></div> : <div className="public-empty"><strong>当前筛选暂无科研数据</strong><p>倾向未识别的历史记录包含在“全部倾向”中。可放宽筛选后重试。</p></div>}
        </section></>}
        <ItemsTable items={summary.items} />
      </>}
      <aside className="public-method"><h2>统计口径</h2><p>一次上报可以包含多张原图，完成记录、项目槽位和图片张数是不同单位。只有完整识别的事件参与统计；识别失败或无效截图不计入。新模板重识别会替换旧结果，不重复累加。</p><p>项目出现占比是观测中的相对频率，不等同于游戏公开概率。区服、科研倾向、项目期数和采集时段会影响样本分布。每小时均量仅使用含有效时长的样本，缺失时长显示“—”。</p><a href={query || '/api/v2/get/research'}>查看当前查询的 JSON ↗</a></aside>
    </main><footer className="public-footer">AzurPilot · 社区截图统计<span>数据随识别结果更新</span></footer>
  </div>
}
