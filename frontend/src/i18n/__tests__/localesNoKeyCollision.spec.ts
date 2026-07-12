import { readdirSync, readFileSync, statSync } from 'node:fs'

import { describe, expect, it } from 'vitest'

import enAdminAccounts from '../locales/en/admin/accounts'
import enAdminChannels from '../locales/en/admin/channels'
import enAdminCodexInspection from '../locales/en/admin/codexInspection'
import enAdminOps from '../locales/en/admin/ops'
import enAdminOverview from '../locales/en/admin/overview'
import enAdminResources from '../locales/en/admin/resources'
import enAdminSettings from '../locales/en/admin/settings'
import enCommon from '../locales/en/common'
import enDashboard from '../locales/en/dashboard'
import enLanding from '../locales/en/landing'
import enMisc from '../locales/en/misc'
import zhAdminAccounts from '../locales/zh/admin/accounts'
import zhAdminChannels from '../locales/zh/admin/channels'
import zhAdminCodexInspection from '../locales/zh/admin/codexInspection'
import zhAdminOps from '../locales/zh/admin/ops'
import zhAdminOverview from '../locales/zh/admin/overview'
import zhAdminResources from '../locales/zh/admin/resources'
import zhAdminSettings from '../locales/zh/admin/settings'
import zhCommon from '../locales/zh/common'
import zhDashboard from '../locales/zh/dashboard'
import zhLocale from '../locales/zh'
import zhLanding from '../locales/zh/landing'
import zhMisc from '../locales/zh/misc'
import enLocale from '../locales/en'

// locales/{zh,en}/index.ts 与 admin/index.ts 使用对象展开聚合各域模块，
// 展开模块之间若出现同名顶层键会静默覆盖。本测试将该风险固化为显式失败。
type Modules = Record<string, Record<string, unknown>>

function collisions(modules: Modules): string[] {
  const seen = new Map<string, string>()
  const out: string[] = []
  for (const [name, mod] of Object.entries(modules)) {
    for (const key of Object.keys(mod)) {
      const prev = seen.get(key)
      if (prev) {
        out.push(`"${key}" in both ${prev} and ${name}`)
      } else {
        seen.set(key, name)
      }
    }
  }
  return out
}

function getPath(root: Record<string, unknown>, path: string): unknown {
  return path.split('.').reduce<unknown>((current, segment) => {
    if (!current || typeof current !== 'object') {
      return undefined
    }
    return (current as Record<string, unknown>)[segment]
  }, root)
}

function literalTranslationKeys(source: string): string[] {
  return Array.from(
    source.matchAll(/(?:\b|\$)t\s*\(\s*(['"`])([A-Za-z][A-Za-z0-9_.-]*\.[A-Za-z0-9_.-]+)\1/g),
    (match) => match[2]
  ).filter((key) => !key.endsWith('.'))
}

function literalMetadataKeys(source: string): string[] {
  return Array.from(
    source.matchAll(/\b(?:titleKey|descriptionKey|messageKey|hintKey|labelKey)\s*:\s*(['"`])([A-Za-z][A-Za-z0-9_.-]*\.[A-Za-z0-9_.-]+)\1/g),
    (match) => match[2]
  ).filter((key) => !key.endsWith('.'))
}

function literalLocaleKeys(source: string, namespace: string): string[] {
  const escaped = namespace.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const pattern = new RegExp(`['"\`](${escaped}\\.[A-Za-z0-9_.-]+)['"\`]`, 'g')
  return Array.from(source.matchAll(pattern), (match) => match[1]).filter((key) => !key.endsWith('.'))
}

function readSources(paths: string[]): string {
  return paths.map((path) => readFileSync(path, 'utf8')).join('\n')
}

function sourceFilePaths(dir: string): string[] {
  const entries = readdirSync(dir, { withFileTypes: true })
  const paths: string[] = []

  for (const entry of entries) {
    const fullPath = `${dir}/${entry.name}`
    if (entry.isDirectory()) {
      if (entry.name === '__tests__' || fullPath === 'src/i18n/locales') {
        continue
      }
      paths.push(...sourceFilePaths(fullPath))
      continue
    }

    if (!statSync(fullPath).isFile()) {
      continue
    }
    if (!/\.(vue|ts|tsx|js|jsx)$/.test(entry.name) || /\.(spec|test)\./.test(entry.name)) {
      continue
    }
    paths.push(fullPath)
  }

  return paths
}

function staticSourceLocaleKeys(): string[] {
  const keys = sourceFilePaths('src').flatMap((path) => {
    const source = readFileSync(path, 'utf8')
    return [
      ...literalTranslationKeys(source),
      ...literalMetadataKeys(source)
    ]
  })

  return [...new Set(keys)].sort()
}

const roots: Record<string, Modules> = {
  zh: { landing: zhLanding, common: zhCommon, dashboard: zhDashboard, misc: zhMisc },
  en: { landing: enLanding, common: enCommon, dashboard: enDashboard, misc: enMisc }
}

const admins: Record<string, Modules> = {
  zh: {
    overview: zhAdminOverview,
    channels: zhAdminChannels,
    accounts: zhAdminAccounts,
    resources: zhAdminResources,
    ops: zhAdminOps,
    settings: zhAdminSettings,
    codexInspection: zhAdminCodexInspection
  },
  en: {
    overview: enAdminOverview,
    channels: enAdminChannels,
    accounts: enAdminAccounts,
    resources: enAdminResources,
    ops: enAdminOps,
    settings: enAdminSettings,
    codexInspection: enAdminCodexInspection
  }
}

describe.each(Object.keys(roots))('locale %s spread assembly', (locale) => {
  it('root modules have no overlapping top-level keys', () => {
    expect(collisions(roots[locale])).toEqual([])
  })

  it('root modules do not shadow the explicit "admin" namespace', () => {
    for (const [name, mod] of Object.entries(roots[locale])) {
      expect(Object.keys(mod), `module ${name} must not define "admin"`).not.toContain('admin')
    }
  })

  it('admin modules have no overlapping top-level keys', () => {
    expect(collisions(admins[locale])).toEqual([])
  })
})

describe('codex inspection locale coverage', () => {
  const viewSource = readFileSync('src/views/admin/CodexInspectionView.vue', 'utf8')
  const staticKeys = literalTranslationKeys(viewSource)
  const dynamicKeys = [
    'admin.codexInspection.action.keep',
    'admin.codexInspection.action.enable',
    'admin.codexInspection.action.disable',
    'admin.codexInspection.action.reauth',
    'admin.codexInspection.action.delete',
    'admin.codexInspection.actionStatus.none',
    'admin.codexInspection.actionStatus.pending',
    'admin.codexInspection.actionStatus.success',
    'admin.codexInspection.actionStatus.failed',
    'admin.codexInspection.actionStatus.skipped',
    'admin.codexInspection.actionStatus.needs_review',
    'admin.codexInspection.probe.success',
    'admin.codexInspection.probe.failed',
    'admin.codexInspection.probe.skipped',
    'nav.codexInspection'
  ]

  it.each([
    ['zh', zhLocale],
    ['en', enLocale]
  ])('%s locale resolves all codex inspection page keys', (_, locale) => {
    for (const key of [...new Set([...staticKeys, ...dynamicKeys])]) {
      expect(getPath(locale, key), key).toBeDefined()
    }
  })
})

describe('admin accounts locale coverage', () => {
  const source = readSources([
    'src/views/admin/AccountsView.vue',
    'src/components/admin/account/AccountActionMenu.vue',
    'src/components/admin/account/AccountTestModal.vue'
  ])
  const staticKeys = [...new Set([
    ...literalTranslationKeys(source).filter((key) => key.startsWith('admin.accounts.')),
    ...literalLocaleKeys(source, 'admin.accounts')
  ])]

  it.each([
    ['zh', zhLocale],
    ['en', enLocale]
  ])('%s locale resolves static admin account page keys', (_, locale) => {
    for (const key of staticKeys) {
      expect(getPath(locale, key), key).toBeDefined()
    }
  })
})

describe('frontend static locale coverage', () => {
  const staticKeys = staticSourceLocaleKeys()

  it.each([
    ['zh', zhLocale],
    ['en', enLocale]
  ])('%s locale resolves static source keys', (_, locale) => {
    for (const key of staticKeys) {
      expect(getPath(locale, key), key).toBeDefined()
    }
  })
})
