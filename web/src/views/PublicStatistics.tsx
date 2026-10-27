/* Constraint brief: primary = real sample counts and project/drop statistics;
   secondary = filters and half-hour comparisons; tertiary = methodology/API links.
   One primary action: apply filters. Reuse existing light CSS tokens: --accent
   #056dff, --text #18181b, --panel, --line, 8px radius, 8/16/24/32px spacing.
   No chart dependency; keyboard-accessible bars keep raw denominators inspectable. */
import { useEffect, useMemo, useState } from 'react'
import { Logo } from '../ui'
import './PublicStatistics.css'

interface Group {
  series: number; focus_series: number; project: string; scene: string; appearances: number; samples: number
  duration_seconds: number; appearance_rate: number; total_appearances: number
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
  slot_index: number; slot_start: string; series: number; focus_series: number; project: string; scene: string
  appearances: number; samples: number; total_appearances: number; appearance_rate: number
}
interface Filters { days: string; server: string; series: string; focus: string; scene: string }
interface Dimension { series: number; focus_series: number; scene: string; events: number; appearances: number; samples: number }
const initial = new URLSearchParams(location.search)
const defaults: Filters = { days: ['1', '7', '30', '90', '365'].includes(initial.get('days') ?? '') ? initial.get('days')! : '30', server: initial.get('server') ?? 'cn', series: initial.get('series') ?? '', focus: initial.get('focus_series') ?? '', scene: initial.get('scene') ?? '' }
const servers = [['', '全部区服'], ['cn', '国服安卓'], ['cn_ios', '国服 iOS'], ['cn_channel', '国服渠道'], ['en', '国际服'], ['jp', '日服'], ['tw', '台服']]
const tags: Record<string, string> = { '': '普通', bonus: '额外', event: '活动', catchup: '追赶', meow: '猫箱', log: '记录', scan: '扫描' }
const scenes: Record<string, string> = {
  commission: '委托奖励', opsi_daily: '大世界每日', opsi_explore: '大世界探索',
  opsi_meowfficer_farming: '大世界猫点', opsi_obscure: '隐秘海域', opsi_abyssal: '深渊海域',
  opsi_stronghold: '大世界要塞', opsi_month_boss: '大世界月度 Boss',
  research_refresh: '科研列表刷新', research_complete: '科研完成',
}
const sceneLabel = (scene: string) => scenes[scene] ?? (scene || '未标注来源')
const focusValue = (focus: number) => focus ? String(focus) : 'unknown'
const focusLabel = (focus: number) => focus ? `倾向第 ${focus} 期` : '倾向未识别'
const number = (n: number) => n.toLocaleString('zh-CN')
const percent = (n: number) => `${(n * 100).toFixed(2)}%`
const groupLabel = (g: Pick<Group, 'series' | 'project'>) => g.series ? `第 ${g.series} 期 · ${g.project}` : g.project || '通用掉落'
const keyOf = (g: Pick<Group, 'series' | 'focus_series' | 'project' | 'scene'>) => JSON.stringify([g.series, g.focus_series, g.project, g.scene])

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
    const rows = slots.filter((s) => s.slot_index === index && selected && s.series === selected.series && s.focus_series === selected.focus_series)
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
    <p className="public-note">北京时间（UTC+8），按截图采集时刻合并各日同一时段。分母仅包含当前期数、当前科研倾向的项目槽位。</p>
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

function ItemsTable({ items, research }: { items: Item[]; research: boolean }) {
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(0)
  const filtered = items.filter((row) => `${row.item} ${row.project} ${row.scene}`.toLowerCase().includes(search.toLowerCase()))
  const pages = Math.max(1, Math.ceil(filtered.length / 25))
  const current = Math.min(page, pages - 1)
  return <section className="public-card">
    <div className="public-section-head"><div><p className="public-eyebrow">REWARDS</p><h2>{research ? '科研完成奖励' : '来源掉落明细'}</h2></div><label>搜索物品 / 项目<input type="search" value={search} onChange={(e) => { setSearch(e.target.value); setPage(0) }} placeholder="例如 Blueprint、Coins" /></label></div>
    <p className="public-note">{research ? '当前期数下，按项目和倾向分别列行统计。' : '仅统计当前选中的掉落来源。'}普通、额外等标签分开计数；单次均量包含未掉落该物品的有效样本。</p>
    {filtered.length ? <><div className="table-wrap"><table className="public-table"><thead><tr><th>物品</th><th>项目 / 场景</th><th>总数量</th><th>掉落样本 / 有效样本</th><th>掉落率</th><th>单次均量</th><th>每小时</th></tr></thead><tbody>
      {filtered.slice(current * 25, (current + 1) * 25).map((row) => <tr key={JSON.stringify([keyOf(row), row.item, row.tag])}><td><strong>{row.item}</strong><span className="public-cell-note">{tags[row.tag] ?? row.tag}</span></td><td>{research ? groupLabel(row) : sceneLabel(row.scene)}<span className="public-cell-note">{research ? focusLabel(row.focus_series) : row.scene || '未标注来源'}</span></td><td>{number(row.amount)}</td><td>{number(row.occurrences)} / {number(row.samples)}</td><td>{percent(row.drop_rate)}</td><td>{row.average_per_sample.toFixed(3)}</td><td>{row.amount_per_hour === null ? '—' : row.amount_per_hour.toFixed(3)}</td></tr>)}
    </tbody></table></div><div className="public-pagination"><span>{number(filtered.length)} 条 · 第 {current + 1} / {pages} 页</span><div><button className="ghost" disabled={current === 0} onClick={() => setPage(current - 1)}>上一页</button><button className="ghost" disabled={current + 1 >= pages} onClick={() => setPage(current + 1)}>下一页</button></div></div></> : <div className="public-empty"><strong>{search ? '没有符合搜索的物品' : '当前筛选暂无掉落数据'}</strong><p>仅识别完整的结果计入统计。识别失败的截图会在模板补齐并重识别后更新。</p></div>}
  </section>
}

export function PublicStatistics() {
  const [view, setView] = useState<'research' | 'drop'>(() => new URLSearchParams(location.search).get('view') === 'drop' ? 'drop' : 'research')
  const [draft, setDraft] = useState<Filters>(defaults)
  const [applied, setApplied] = useState<Filters>(defaults)
  const [tick, setTick] = useState(0)
  const [data, setData] = useState<{ summary: Summary; slots: Slot[]; options: Dimension[]; selection: Filters } | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [query, setQuery] = useState('')
  useEffect(() => { document.title = 'AzurPilot 统计 · 科研与掉落' }, [])
  useEffect(() => {
    const controller = new AbortController()
    setLoading(true); setError(''); setData(null); setQuery('')
    const to = new Date()
    const params = new URLSearchParams({ from: new Date(to.getTime() - Number(applied.days) * 86400000).toISOString(), to: to.toISOString(), timezone: 'Asia/Shanghai' })
    if (applied.server) params.set('server', applied.server)
    async function load() {
      const { options } = await readPublic<{ options: Dimension[] }>(`/api/v2/get/${view}/options?${params}`, controller.signal)
      const selection = { ...applied }
      if (view === 'research') {
        const periods = [...new Set(options.map((o) => o.series))].sort((a, b) => b - a)
        selection.series = periods.includes(Number(applied.series)) ? applied.series : String(periods[0] ?? '')
        const focuses = [...new Set(options.filter((o) => String(o.series) === selection.series && o.appearances > 0).map((o) => o.focus_series))].sort((a, b) => b - a)
        selection.focus = focuses.map(focusValue).includes(applied.focus) ? applied.focus : focuses.length ? focusValue(focuses[0]) : ''
        if (selection.series) params.set('series', selection.series)
        selection.scene = ''
      } else {
        const sources = [...new Set(options.map((o) => o.scene))]
        selection.scene = sources.includes(applied.scene) ? applied.scene : sources.includes('opsi_daily') ? 'opsi_daily' : sources[0] ?? ''
        if (selection.scene) params.set('scene', selection.scene)
      }
      const path = `/api/v2/get/${view}?${params}`
      const slotParams = new URLSearchParams(params)
      if (view === 'research' && selection.focus) slotParams.set('focus_series', selection.focus)
      const [summary, times] = await Promise.all([
        readPublic<Summary>(path, controller.signal),
        view === 'research' ? readPublic<{ slots: Slot[] }>(`/api/v2/get/research/time-slots?${slotParams}`, controller.signal) : Promise.resolve({ slots: [] }),
      ])
      if (!controller.signal.aborted) {
        setQuery(path)
        setData({ summary, slots: times.slots, options, selection })
        const bookmark = new URLSearchParams({ view, days: selection.days, server: selection.server })
        if (view === 'research') {
          if (selection.series) bookmark.set('series', selection.series)
          if (selection.focus) bookmark.set('focus_series', selection.focus)
        } else if (selection.scene) bookmark.set('scene', selection.scene)
        history.replaceState(null, '', `/?${bookmark}`)
      }
    }
    load()
      .catch((e: Error) => { if (!controller.signal.aborted) setError(e.message) })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [view, applied, tick])
  const summary = data?.summary
  const samples = summary?.groups.reduce((sum, g) => sum + g.samples, 0) ?? 0
  const itemCount = new Set(summary?.items.map((i) => i.item)).size
  const totalAmount = summary?.items.reduce((sum, i) => sum + i.amount, 0) ?? 0
  const selection = data?.selection ?? applied
  const periods = [...new Set(data?.options.map((o) => o.series))].sort((a, b) => b - a)
  const focuses = [...new Set(data?.options.filter((o) => String(o.series) === selection.series && o.appearances > 0).map((o) => o.focus_series))].sort((a, b) => b - a)
  const sources = [...new Set(data?.options.map((o) => o.scene))].sort()
  const observed = summary?.groups.filter((g) => g.appearances > 0 && focusValue(g.focus_series) === selection.focus) ?? []
  const appearances = observed.reduce((sum, g) => sum + g.appearances, 0)
  const completed = summary?.groups.filter((g) => g.samples > 0) ?? []
  const scope = view === 'research' ? selection.series ? `第 ${selection.series} 期 · ${selection.focus ? selection.focus === 'unknown' ? '倾向未识别' : `倾向第 ${selection.focus} 期` : '暂无刷新观测'}` : '暂无科研记录' : sceneLabel(selection.scene)
  const chooseDimension = (change: Partial<Filters>) => setApplied({ ...selection, ...change })
  const switchView = (next: 'research' | 'drop') => { setView(next); setApplied({ ...applied, series: '', focus: '', scene: '' }) }
  return <div className="public-site">
    <header className="public-topbar"><a className="public-brand" href="/"><Logo size={32} /><strong>AzurPilot <span>统计</span></strong></a><nav aria-label="网站导航"><a href="/admin/">管理后台 ↗</a></nav></header>
    <main className="public-main">
      <div className="public-intro"><div><p className="public-eyebrow">COMMUNITY STATISTICS</p><h1>让每一份记录，都有参考价值。</h1><p>汇总社区上传的科研与掉落截图，观察项目分布和时段差异。</p></div><span className="public-badge">公开查询 · 无需登录</span></div>
      <nav className="public-tabs" aria-label="统计类型"><button className={view === 'research' ? 'active' : ''} aria-current={view === 'research' ? 'page' : undefined} onClick={() => switchView('research')}>科研统计</button><button className={view === 'drop' ? 'active' : ''} aria-current={view === 'drop' ? 'page' : undefined} onClick={() => switchView('drop')}>通用掉落</button></nav>
      <form className="public-filters" onSubmit={(e) => { e.preventDefault(); setApplied({ ...selection, days: draft.days, server: draft.server }); setTick((n) => n + 1) }}>
        <label>统计区间<select value={draft.days} onChange={(e) => setDraft({ ...draft, days: e.target.value })}>{[1, 7, 30, 90, 365].map((d) => <option key={d} value={d}>最近 {d} 天</option>)}</select></label>
        <label>区服<select value={draft.server} onChange={(e) => setDraft({ ...draft, server: e.target.value })}>{servers.map(([id, title]) => <option key={id} value={id}>{title}</option>)}</select></label>
        <button type="submit" disabled={loading}>{loading ? '查询中…' : '应用筛选'}</button>
      </form>
      <div role="status" className="public-query-status">{loading ? '正在读取最新统计…' : summary ? `数据截至 ${new Date(summary.to).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false })}（北京时间）` : '查询未完成'}<span>每 30 分钟派发新截图；重识别结果回传后更新</span></div>
      {error && <div className="public-error" role="alert"><strong>暂时无法取得统计</strong><p>{error}</p><button className="ghost" onClick={() => setTick((n) => n + 1)}>重试查询</button></div>}
      {loading && <div className="public-loading" aria-label="统计加载中"><span /><span /><span /></div>}
      {summary && <>
        <section className="public-card public-dimensions" aria-label={view === 'research' ? '科研期数和倾向' : '掉落来源'}>
          <div className="public-section-head"><div><p className="public-eyebrow">{view === 'research' ? 'RESEARCH SERIES' : 'DROP SOURCES'}</p><h2>{view === 'research' ? '按期查看科研' : '按来源查看掉落'}</h2></div>{view === 'research' && focuses.length > 0 && <label>刷新科研倾向<select value={selection.focus} onChange={(e) => chooseDimension({ focus: e.target.value })}>{focuses.map((f) => <option key={f} value={focusValue(f)}>{focusLabel(f)}</option>)}</select></label>}</div>
          <div className="public-dimension-tabs" role="group" aria-label={view === 'research' ? '选择科研期数' : '选择掉落来源'}>
            {view === 'research' ? periods.map((p) => <button type="button" className={`ghost ${selection.series === String(p) ? 'selected' : ''}`} aria-pressed={selection.series === String(p)} key={p} onClick={() => chooseDimension({ series: String(p) })}>第 {p} 期</button>) : sources.map((source) => <button type="button" className={`ghost ${selection.scene === source ? 'selected' : ''}`} aria-pressed={selection.scene === source} key={source} onClick={() => chooseDimension({ scene: source })}>{sceneLabel(source)}<span>{source || '未标注来源'}</span></button>)}
          </div>
          <p className="public-scope">{data?.options.length ? <>当前统计：<strong>{scope}</strong>。{view === 'research' ? '倾向筛选用于项目出现和半小时图；完成奖励按本期期数展示，倾向信息分组保留。' : '样本数、掉落率和单次均量均使用此来源。'}</> : '当前区服和统计区间暂无有效记录。'}</p>
        </section>
        <div className="public-metrics">{[
          [view === 'research' ? '本期已观测项目槽位' : '当前来源有效样本', view === 'research' ? appearances : samples, view === 'research' ? '仅包含当前期数、当前倾向的项目槽位' : sceneLabel(selection.scene)],
          [view === 'research' ? '本期科研完成记录' : '累计掉落数量', view === 'research' ? samples : totalAmount, view === 'research' ? '本期各倾向的有效完成记录' : '当前来源内各物品数量之和'],
          ['掉落物品种类', itemCount, view === 'research' ? '当前期数完成奖励中的物品种类' : '当前来源内按物品标识去重'],
        ].map(([title, value, note]) => <section key={title} className="public-metric"><p>{title}</p><strong>{number(Number(value))}</strong><span>{note}</span></section>)}</div>
        {view === 'research' && <><TimeSlots key={scope} slots={data?.slots ?? []} groups={observed} /><section className="public-card"><div className="public-section-head"><div><p className="public-eyebrow">PROJECT APPEARANCES</p><h2>科研项目出现分布</h2></div><span className="public-note">{scope}</span></div>
          <p className="public-note">期内出现占比 = 当前项目出现次数 ÷ 本期、当前倾向的全部已观测项目槽位。完成奖励记录不参与出现次数统计。</p>
          {observed.length ? <div className="table-wrap"><table className="public-table"><thead><tr><th>项目</th><th>出现次数</th><th>期内项目槽位</th><th>期内出现占比</th></tr></thead><tbody>{observed.map((g) => <tr key={keyOf(g)}><td><strong>{groupLabel(g)}</strong><span className="public-cell-note">{sceneLabel(g.scene)}</span></td><td>{number(g.appearances)}</td><td>{number(g.total_appearances)}</td><td><span className="public-rate"><i style={{ width: `${g.appearance_rate * 100}%` }} /></span>{percent(g.appearance_rate)}</td></tr>)}</tbody></table></div> : <div className="public-empty"><strong>当前期数和倾向暂无项目出现观测</strong><p>完成记录单独列在下方；缺少刷新观测时不推算出现率。</p></div>}
        </section><section className="public-card"><div className="public-section-head"><div><p className="public-eyebrow">COMPLETED PROJECTS</p><h2>科研完成记录</h2></div><span className="public-note">{selection.series ? `第 ${selection.series} 期` : '暂无科研记录'}</span></div>
          <p className="public-note">完成截图可能未包含科研倾向，未识别的记录保留在本期完成统计中。不同倾向的完成记录各自列行。</p>
          {completed.length ? <div className="table-wrap"><table className="public-table"><thead><tr><th>项目</th><th>倾向</th><th>完成样本</th><th>记录总时长</th></tr></thead><tbody>{completed.map((g) => <tr key={keyOf(g)}><td><strong>{groupLabel(g)}</strong></td><td>{focusLabel(g.focus_series)}</td><td>{number(g.samples)}</td><td>{g.duration_seconds > 0 ? `${(g.duration_seconds / 3600).toFixed(1)} h` : '—'}</td></tr>)}</tbody></table></div> : <div className="public-empty"><strong>当前期数暂无科研完成记录</strong></div>}
        </section></>}
        <ItemsTable key={JSON.stringify([view, selection])} items={summary.items} research={view === 'research'} />
      </>}
      <aside className="public-method"><h2>统计口径</h2><p>一次上报可以包含多张原图，完成记录、项目槽位和图片张数是不同单位。只有完整识别的事件参与统计；识别失败或无效截图不计入。新模板重识别会替换旧结果，不重复累加。</p><p>科研按期数和倾向分别查看，默认显示有记录的最新一期；倾向未识别的记录单列。掉落按来源分别查看，委托与各类大世界奖励独立统计。项目出现占比是期内观测的相对频率，不等同于游戏公开概率。每小时均量仅使用含有效时长的样本，缺失时长显示“—”。</p><a href={query || '/api/v2/get/research'}>查看当前查询的 JSON ↗</a></aside>
    </main><footer className="public-footer">AzurPilot · 社区截图统计<span>数据随识别结果更新</span></footer>
  </div>
}
