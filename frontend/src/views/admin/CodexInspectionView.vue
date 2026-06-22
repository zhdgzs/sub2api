<template>
  <AppLayout>
    <div class="min-h-screen bg-gray-50 px-4 py-5 dark:bg-dark-950 sm:px-6 lg:px-8">
      <div class="mx-auto max-w-[1600px] space-y-5">
        <div class="flex flex-col gap-3 rounded-lg border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-900 lg:flex-row lg:items-center lg:justify-between">
          <div>
            <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('admin.codexInspection.title') }}</h1>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.codexInspection.description') }}</p>
          </div>
          <div class="flex flex-wrap gap-2">
            <button class="btn-primary" :disabled="busy || !!runningRun" @click="startInspection">
              <Icon name="play" size="sm" />
              <span>{{ t('admin.codexInspection.actions.runNow') }}</span>
            </button>
            <button class="btn-secondary" :disabled="!selectedResults.length || busy || !!runningRun" @click="inspectSelected">
              <Icon name="beaker" size="sm" />
              <span>{{ t('admin.codexInspection.actions.inspectSelected') }}</span>
            </button>
            <button class="btn-secondary" :disabled="!selectedResults.length || busy" @click="applySelected">
              <Icon name="check" size="sm" />
              <span>{{ t('admin.codexInspection.actions.applySelected') }}</span>
            </button>
            <button class="btn-secondary" :disabled="!runningRun || busy" @click="cancelRunning">
              <Icon name="x" size="sm" />
              <span>{{ t('admin.codexInspection.actions.cancel') }}</span>
            </button>
            <button class="btn-secondary" :disabled="busy" @click="reload()">
              <Icon name="refresh" size="sm" />
              <span>{{ t('common.refresh') }}</span>
            </button>
            <button class="btn-secondary" :disabled="!latestResults.length" @click="exportCSV">
              <Icon name="download" size="sm" />
              <span>{{ t('admin.codexInspection.actions.exportCsv') }}</span>
            </button>
            <button class="btn-secondary" @click="activeTab = 'settings'">
              <Icon name="cog" size="sm" />
              <span>{{ t('admin.codexInspection.tabs.settings') }}</span>
            </button>
            <button class="btn-secondary" @click="activeTab = 'logs'">
              <Icon name="document" size="sm" />
              <span>{{ t('admin.codexInspection.tabs.logs') }}</span>
            </button>
          </div>
        </div>

        <div v-if="runningRun" class="rounded-lg border border-blue-200 bg-blue-50 p-4 dark:border-blue-900/60 dark:bg-blue-950/30">
          <div class="flex items-center justify-between gap-3 text-sm text-blue-900 dark:text-blue-100">
            <span>Run #{{ runningRun.id }} {{ runningRun.completed_accounts }}/{{ runningRun.total_accounts }}</span>
            <span>{{ progressPercent }}%</span>
          </div>
          <div class="mt-2 h-2 overflow-hidden rounded-full bg-blue-100 dark:bg-blue-900">
            <div class="h-full rounded-full bg-blue-600 transition-all" :style="{ width: `${progressPercent}%` }"></div>
          </div>
        </div>

        <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
          <div v-for="item in summaryCards" :key="item.key" class="rounded-lg border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-900">
            <div class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">{{ item.label }}</div>
            <div class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ item.value }}</div>
          </div>
        </div>

        <div class="rounded-lg border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
          <div class="flex flex-wrap border-b border-gray-200 px-3 dark:border-dark-700">
            <button v-for="tab in tabs" :key="tab.key" class="tab-button" :class="{ 'tab-button-active': activeTab === tab.key }" @click="activeTab = tab.key">
              {{ tab.label }}
            </button>
          </div>

          <section v-if="activeTab === 'accounts'" class="p-4">
            <div v-if="currentRunResultsId" class="mb-4 flex flex-wrap items-center justify-between gap-3 rounded-md border border-blue-200 bg-blue-50 px-3 py-2 text-sm text-blue-900 dark:border-blue-900/60 dark:bg-blue-950/30 dark:text-blue-100">
              <span>{{ t('admin.codexInspection.viewingRunResults', { id: currentRunResultsId }) }}</span>
              <button class="btn-secondary" @click="clearRunResults">{{ t('admin.codexInspection.actions.backToLatest') }}</button>
            </div>
            <div class="mb-4 grid gap-3 lg:grid-cols-8">
              <input v-model.trim="filters.search" class="form-input lg:col-span-2" :placeholder="t('admin.codexInspection.filters.search')" @keyup.enter="loadLatest" />
              <select v-model="filters.action" class="form-input" @change="loadLatest">
                <option value="">{{ t('admin.codexInspection.filters.allActions') }}</option>
                <option v-for="action in actionOptions" :key="action" :value="action">{{ actionLabel(action) }}</option>
              </select>
              <select v-model="filters.probe_status" class="form-input" @change="loadLatest">
                <option value="">{{ t('admin.codexInspection.filters.allProbeStatus') }}</option>
                <option value="success">{{ t('admin.codexInspection.probe.success') }}</option>
                <option value="failed">{{ t('admin.codexInspection.probe.failed') }}</option>
                <option value="skipped">{{ t('admin.codexInspection.probe.skipped') }}</option>
              </select>
              <select v-model="filters.account_status" class="form-input" @change="loadLatest">
                <option value="">{{ t('admin.codexInspection.filters.allAccountStatus') }}</option>
                <option value="active">{{ t('admin.codexInspection.status.active') }}</option>
                <option value="error">{{ t('admin.codexInspection.status.error') }}</option>
              </select>
              <select v-model="filters.quota_window" class="form-input" @change="loadLatest">
                <option value="">{{ t('admin.codexInspection.filters.allQuota') }}</option>
                <option value="normal">{{ t('admin.codexInspection.filters.quotaNormal') }}</option>
                <option value="five_full">{{ t('admin.codexInspection.filters.fiveFull') }}</option>
                <option value="long_full">{{ t('admin.codexInspection.filters.longFull') }}</option>
              </select>
              <input v-model.trim="filters.group_ids_text" class="form-input" :placeholder="t('admin.codexInspection.filters.groupIds')" @keyup.enter="loadLatest" />
              <input v-model.number="filters.only_stale_minutes" class="form-input" type="number" min="0" :placeholder="t('admin.codexInspection.filters.onlyStale')" @keyup.enter="loadLatest" />
              <button class="btn-secondary justify-center" @click="loadLatest">
                <Icon name="search" size="sm" />
                <span>{{ t('common.search') }}</span>
              </button>
            </div>

            <div class="overflow-x-auto">
              <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
                <thead class="bg-gray-50 text-left text-xs font-semibold uppercase text-gray-500 dark:bg-dark-800 dark:text-gray-400">
                  <tr>
                    <th class="w-10 px-3 py-3"><input type="checkbox" :checked="allVisibleSelected" @change="toggleAllVisible" /></th>
                    <th class="px-3 py-3">{{ t('admin.codexInspection.columns.account') }}</th>
                    <th class="px-3 py-3">{{ t('admin.codexInspection.columns.identity') }}</th>
                    <th class="px-3 py-3">{{ t('admin.codexInspection.columns.status') }}</th>
                    <th class="px-3 py-3">5h</th>
                    <th class="px-3 py-3">{{ t('admin.codexInspection.columns.longWindow') }}</th>
                    <th class="px-3 py-3">{{ t('admin.codexInspection.columns.upstream') }}</th>
                    <th class="px-3 py-3">{{ t('admin.codexInspection.columns.action') }}</th>
                    <th class="px-3 py-3">{{ t('admin.codexInspection.columns.actionStatus') }}</th>
                    <th class="min-w-[260px] px-3 py-3">{{ t('admin.codexInspection.columns.reason') }}</th>
                    <th class="px-3 py-3">{{ t('admin.codexInspection.columns.checkedAt') }}</th>
                    <th class="px-3 py-3">{{ t('admin.codexInspection.columns.ops') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
                  <tr v-for="row in latestResults" :key="row.id" class="hover:bg-gray-50 dark:hover:bg-dark-800/70">
                    <td class="px-3 py-3"><input type="checkbox" :value="row.id" v-model="selectedResults" /></td>
                    <td class="px-3 py-3">
                      <div class="font-medium text-gray-900 dark:text-white">{{ row.account_name || `#${row.account_id}` }}</div>
                      <div class="text-xs text-gray-500">#{{ row.account_id }}</div>
                    </td>
                    <td class="px-3 py-3">
                      <div class="text-xs text-gray-600 dark:text-gray-300">Proxy {{ row.proxy_id_snapshot || '-' }}</div>
                      <div class="max-w-[180px] truncate text-xs text-gray-500" :title="row.chatgpt_account_id || '-'">ChatGPT {{ row.chatgpt_account_id || '-' }}</div>
                    </td>
                    <td class="px-3 py-3">
                      <div class="flex flex-col gap-1">
                        <span class="badge" :class="probeClass(row.probe_status)">{{ probeLabel(row.probe_status) }}</span>
                        <span class="text-xs text-gray-500">{{ row.account_status_snapshot }} / {{ row.schedulable_snapshot ? 'schedulable' : 'paused' }}</span>
                      </div>
                    </td>
                    <td class="px-3 py-3">{{ percent(row.five_hour_used_percent) }}</td>
                    <td class="px-3 py-3">{{ longWindowText(row) }}</td>
                    <td class="px-3 py-3">{{ row.upstream_status_code || '-' }}</td>
                    <td class="px-3 py-3"><span class="badge" :class="actionClass(row.recommended_action)">{{ actionLabel(row.recommended_action) }}</span></td>
                    <td class="px-3 py-3"><span class="badge" :class="actionStatusClass(row.action_status)">{{ actionStatusLabel(row.action_status) }}</span></td>
                    <td class="px-3 py-3 text-gray-600 dark:text-gray-300">{{ row.action_reason }}</td>
                    <td class="px-3 py-3 whitespace-nowrap">{{ formatDate(row.created_at) }}</td>
                    <td class="px-3 py-3">
                      <div class="relative flex flex-wrap gap-1.5">
                        <button class="icon-btn" :title="t('admin.codexInspection.actions.probe')" @click="probe(row)"><Icon name="refresh" size="sm" /></button>
                        <button class="icon-btn" :title="t('admin.codexInspection.actions.apply')" @click="applyResult(row)"><Icon name="check" size="sm" /></button>
                        <button class="icon-btn" :title="t('admin.codexInspection.actions.enable')" @click="applyResult(row, 'enable')"><Icon name="checkCircle" size="sm" /></button>
                        <button class="icon-btn" :title="t('admin.codexInspection.actions.disable')" @click="applyResult(row, 'disable')"><Icon name="ban" size="sm" /></button>
                        <button class="icon-btn" :title="t('admin.codexInspection.actions.markReauth')" @click="applyResult(row, 'reauth')"><Icon name="login" size="sm" /></button>
                        <button class="icon-btn" :title="t('common.view')" @click="detail = row"><Icon name="eye" size="sm" /></button>
                        <button class="icon-btn" :title="t('admin.codexInspection.actions.openAccount')" @click="openAccount(row)"><Icon name="externalLink" size="sm" /></button>
                        <button class="icon-btn" :title="t('admin.codexInspection.actions.more')" @click="toggleMore(row.id)"><Icon name="more" size="sm" /></button>
                        <div v-if="moreMenuRowID === row.id" class="more-menu">
                          <button class="more-menu-item danger" @click="openDelete(row)">
                            <Icon name="trash" size="sm" />
                            <span>{{ t('common.delete') }}</span>
                          </button>
                        </div>
                      </div>
                    </td>
                  </tr>
                  <tr v-if="!latestResults.length && !busy">
                    <td colspan="12" class="px-3 py-10 text-center text-gray-500">{{ t('admin.codexInspection.empty') }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div class="mt-4 flex items-center justify-between text-sm text-gray-500">
              <span>{{ latestTotal }}</span>
              <div class="flex gap-2">
                <button class="btn-secondary" :disabled="latestPage <= 1" @click="changeLatestPage(-1)">{{ t('admin.codexInspection.pagination.previous') }}</button>
                <button class="btn-secondary" :disabled="latestPage >= latestPages" @click="changeLatestPage(1)">{{ t('admin.codexInspection.pagination.next') }}</button>
              </div>
            </div>
          </section>

          <section v-else-if="activeTab === 'runs'" class="p-4">
            <div class="overflow-x-auto">
              <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
                <thead class="bg-gray-50 text-left text-xs font-semibold uppercase text-gray-500 dark:bg-dark-800 dark:text-gray-400">
                  <tr>
                    <th class="px-3 py-3">Run ID</th>
                    <th class="px-3 py-3">{{ t('admin.codexInspection.columns.trigger') }}</th>
                    <th class="px-3 py-3">{{ t('admin.codexInspection.columns.status') }}</th>
                    <th class="px-3 py-3">{{ t('admin.codexInspection.columns.total') }}</th>
                    <th class="px-3 py-3">{{ t('admin.codexInspection.columns.resultCounts') }}</th>
                    <th class="px-3 py-3">{{ t('admin.codexInspection.columns.startedAt') }}</th>
                    <th class="px-3 py-3">{{ t('admin.codexInspection.columns.finishedAt') }}</th>
                    <th class="px-3 py-3">{{ t('admin.codexInspection.columns.duration') }}</th>
                    <th class="px-3 py-3">{{ t('admin.codexInspection.columns.ops') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
                  <tr v-for="run in runs" :key="run.id">
                    <td class="px-3 py-3 font-medium">#{{ run.id }}</td>
                    <td class="px-3 py-3">{{ run.trigger_type }}</td>
                    <td class="px-3 py-3"><span class="badge" :class="runClass(run.status)">{{ run.status }}</span></td>
                    <td class="px-3 py-3">{{ run.completed_accounts }}/{{ run.total_accounts }}</td>
                    <td class="px-3 py-3">K {{ run.keep_count }} / E {{ run.enable_count }} / D {{ run.disable_count }} / R {{ run.reauth_count }} / Del {{ run.delete_count }}</td>
                    <td class="px-3 py-3">{{ formatDate(run.started_at) }}</td>
                    <td class="px-3 py-3">{{ formatDate(run.finished_at) }}</td>
                    <td class="px-3 py-3">{{ runDuration(run) }}</td>
                    <td class="px-3 py-3">
                      <div class="flex flex-wrap gap-2">
                        <button class="btn-secondary" @click="viewRun(run)">{{ t('admin.codexInspection.actions.viewResults') }}</button>
                        <button class="btn-secondary" :disabled="busy || !!runningRun" @click="rerunSame(run)">{{ t('admin.codexInspection.actions.rerunSame') }}</button>
                        <button class="btn-secondary" @click="rerunFailed(run)">{{ t('admin.codexInspection.actions.rerunFailed') }}</button>
                        <button class="btn-secondary" @click="exportRun(run)">{{ t('admin.codexInspection.actions.exportRun') }}</button>
                        <button class="btn-secondary" @click="snapshotRun = run">{{ t('admin.codexInspection.actions.viewSnapshot') }}</button>
                      </div>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </section>

          <section v-else-if="activeTab === 'logs'" class="p-4">
            <div class="mb-4 flex flex-wrap gap-2">
              <input v-model.number="logFilters.run_id" class="form-input w-40" placeholder="Run ID" @keyup.enter="loadLogs" />
              <select v-model="logFilters.level" class="form-input w-40" @change="loadLogs">
                <option value="">{{ t('admin.codexInspection.filters.allLevels') }}</option>
                <option value="info">info</option>
                <option value="warning">warning</option>
                <option value="error">error</option>
              </select>
              <button class="btn-secondary" @click="loadLogs"><Icon name="refresh" size="sm" />{{ t('common.refresh') }}</button>
            </div>
            <div class="space-y-2">
              <div v-for="log in logs" :key="log.id" class="rounded-md border border-gray-200 p-3 text-sm dark:border-dark-700">
                <div class="flex items-center justify-between gap-3">
                  <span class="font-medium" :class="log.level === 'error' ? 'text-red-600' : 'text-gray-900 dark:text-white'">{{ log.level }} · Run #{{ log.run_id }}</span>
                  <span class="text-xs text-gray-500">{{ formatDate(log.created_at) }}</span>
                </div>
                <p class="mt-1 text-gray-600 dark:text-gray-300">{{ log.message }}</p>
              </div>
            </div>
          </section>

          <section v-else class="p-4">
            <div v-if="settingsDraft" class="grid gap-5 xl:grid-cols-2">
              <div class="settings-panel">
                <h2>{{ t('admin.codexInspection.settings.schedule') }}</h2>
                <label class="toggle-row"><input type="checkbox" v-model="settingsDraft.enabled" />{{ t('admin.codexInspection.settings.enabled') }}</label>
                <label class="field-label">{{ t('admin.codexInspection.settings.mode') }}</label>
                <select v-model="settingsDraft.schedule.mode" class="form-input">
                  <option value="interval">interval</option>
                  <option value="time_points">time_points</option>
                </select>
                <label class="field-label">{{ t('admin.codexInspection.settings.interval') }}</label>
                <input v-model.number="settingsDraft.schedule.interval_minutes" class="form-input" type="number" min="1" />
                <label class="field-label">{{ t('admin.codexInspection.settings.timePoints') }}</label>
                <input v-model="timePointsText" class="form-input" placeholder="09:00,18:00" @blur="syncTimePoints" />
                <label class="field-label">Timezone</label>
                <input v-model="settingsDraft.schedule.timezone" class="form-input" />
              </div>
              <div class="settings-panel">
                <h2>{{ t('admin.codexInspection.settings.probe') }}</h2>
                <label class="field-label">Workers</label>
                <input v-model.number="settingsDraft.probe.workers" class="form-input" type="number" min="1" max="32" />
                <label class="field-label">Timeout (ms)</label>
                <input v-model.number="settingsDraft.probe.timeout_ms" class="form-input" type="number" min="1000" />
                <label class="field-label">Retries</label>
                <input v-model.number="settingsDraft.probe.retries" class="form-input" type="number" min="0" max="3" />
                <label class="field-label">{{ t('admin.codexInspection.settings.minInterval') }}</label>
                <input v-model.number="settingsDraft.probe.min_interval_minutes" class="form-input" type="number" min="0" />
                <label class="field-label">User-Agent</label>
                <input v-model="settingsDraft.probe.user_agent" class="form-input" :placeholder="t('admin.codexInspection.settings.defaultUa')" />
              </div>
              <div class="settings-panel">
                <h2>{{ t('admin.codexInspection.settings.target') }}</h2>
                <label class="toggle-row"><input type="checkbox" checked disabled />{{ t('admin.codexInspection.settings.onlyOauth') }}</label>
                <label class="toggle-row"><input type="checkbox" v-model="settingsDraft.target.include_unschedulable" />{{ t('admin.codexInspection.settings.includeUnschedulable') }}</label>
                <label class="toggle-row"><input type="checkbox" v-model="settingsDraft.target.include_error" />{{ t('admin.codexInspection.settings.includeError') }}</label>
                <label class="field-label">{{ t('admin.codexInspection.settings.onlyStale') }}</label>
                <input v-model.number="settingsDraft.target.only_stale_minutes" class="form-input" type="number" min="0" />
                <label class="field-label">{{ t('admin.codexInspection.settings.sampleSize') }}</label>
                <input v-model.number="settingsDraft.target.sample_size" class="form-input" type="number" min="0" />
                <label class="field-label">{{ t('admin.codexInspection.settings.groupIds') }}</label>
                <input v-model="targetGroupIdsText" class="form-input" placeholder="1,2,3" @blur="syncTargetIds" />
                <label class="field-label">{{ t('admin.codexInspection.settings.accountIds') }}</label>
                <input v-model="targetAccountIdsText" class="form-input" placeholder="101,102" @blur="syncTargetIds" />
              </div>
              <div class="settings-panel">
                <h2>{{ t('admin.codexInspection.settings.actions') }}</h2>
                <label class="field-label">{{ t('admin.codexInspection.settings.threshold') }}</label>
                <input v-model.number="settingsDraft.decision.used_percent_threshold" class="form-input" type="number" min="1" />
                <label class="field-label">{{ t('admin.codexInspection.settings.shortWindowPolicy') }}</label>
                <select v-model="settingsDraft.decision.short_window_policy" class="form-input" disabled>
                  <option value="keep">{{ actionLabel('keep') }}</option>
                </select>
                <label class="field-label">{{ t('admin.codexInspection.settings.longWindowPolicy') }}</label>
                <select v-model="settingsDraft.decision.long_window_policy" class="form-input">
                  <option value="disable">{{ actionLabel('disable') }}</option>
                  <option value="keep">{{ actionLabel('keep') }}</option>
                </select>
                <label class="toggle-row"><input type="checkbox" v-model="settingsDraft.actions.auto_apply" />{{ t('admin.codexInspection.settings.autoApply') }}</label>
                <label class="toggle-row"><input type="checkbox" v-model="settingsDraft.actions.allow_enable" />{{ t('admin.codexInspection.settings.allowEnable') }}</label>
                <label class="toggle-row"><input type="checkbox" v-model="settingsDraft.actions.allow_disable" />{{ t('admin.codexInspection.settings.allowDisable') }}</label>
                <label class="toggle-row"><input type="checkbox" v-model="settingsDraft.actions.allow_mark_reauth" />{{ t('admin.codexInspection.settings.allowReauth') }}</label>
                <label class="toggle-row opacity-60"><input type="checkbox" disabled />{{ t('admin.codexInspection.settings.allowDelete') }}</label>
              </div>
            </div>
            <div class="mt-5 flex flex-wrap gap-2">
              <button class="btn-primary" :disabled="busy" @click="saveSettings"><Icon name="check" size="sm" />{{ t('common.save') }}</button>
              <button class="btn-secondary" :disabled="busy" @click="resetSettingsDraft">{{ t('admin.codexInspection.actions.resetDefaults') }}</button>
              <button class="btn-secondary" :disabled="busy || !!runningRun" @click="saveAndRun">{{ t('admin.codexInspection.actions.saveAndRun') }}</button>
            </div>
          </section>
        </div>
      </div>
    </div>

    <div v-if="detail" class="fixed inset-0 z-50 flex justify-end bg-black/40" @click.self="detail = null">
      <div class="h-full w-full max-w-4xl overflow-auto bg-white p-5 shadow-xl dark:bg-dark-900">
        <div class="flex items-center justify-between">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ detail.account_name }}</h2>
          <button class="icon-btn" @click="detail = null"><Icon name="x" size="sm" /></button>
        </div>
        <div class="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <div class="detail-box">Run #{{ detail.run_id }}</div>
          <div class="detail-box">{{ probeLabel(detail.probe_status) }} · {{ detail.upstream_status_code || '-' }}</div>
          <div class="detail-box">{{ t('admin.codexInspection.detail.latency') }} {{ detail.latency_ms ?? '-' }}ms</div>
          <div class="detail-box">5h {{ percent(detail.five_hour_used_percent) }}</div>
          <div class="detail-box">{{ longWindowText(detail) }}</div>
          <div class="detail-box">{{ t('admin.codexInspection.detail.proxy') }} {{ detail.proxy_id_snapshot || '-' }}</div>
          <div class="detail-box lg:col-span-2">{{ t('admin.codexInspection.detail.chatgptAccountId') }} {{ detail.chatgpt_account_id || '-' }}</div>
          <div class="detail-box">{{ t('admin.codexInspection.columns.actionStatus') }} {{ actionStatusLabel(detail.action_status) }}</div>
        </div>
        <div class="mt-4 flex flex-wrap gap-2">
          <button class="btn-primary" @click="applyResult(detail)"><Icon name="check" size="sm" />{{ t('admin.codexInspection.actions.apply') }}</button>
          <button class="btn-secondary" @click="probe(detail)"><Icon name="refresh" size="sm" />{{ t('admin.codexInspection.actions.probe') }}</button>
          <button class="btn-secondary" @click="openAccount(detail)"><Icon name="externalLink" size="sm" />{{ t('admin.codexInspection.actions.openAccount') }}</button>
        </div>
        <h3 class="mt-5 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.codexInspection.columns.reason') }}</h3>
        <p class="mt-2 rounded-md bg-gray-50 p-3 text-sm text-gray-700 dark:bg-dark-800 dark:text-gray-300">{{ detail.action_reason }}</p>
        <h3 class="mt-5 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.codexInspection.detail.actionRecord') }}</h3>
        <div class="mt-2 grid gap-3 sm:grid-cols-2">
          <div class="detail-box">{{ t('admin.codexInspection.columns.action') }} {{ actionLabel(detail.recommended_action) }}</div>
          <div class="detail-box">{{ t('admin.codexInspection.columns.actionStatus') }} {{ actionStatusLabel(detail.action_status) }}</div>
          <div class="detail-box sm:col-span-2">{{ detail.action_error || '-' }}</div>
        </div>
        <h3 class="mt-5 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.codexInspection.detail.rawRateLimit') }}</h3>
        <pre class="mt-2 max-h-52 overflow-auto whitespace-pre-wrap rounded-md bg-gray-950 p-3 text-xs text-gray-100">{{ formatJSON(detail.raw_rate_limit) }}</pre>
        <h3 class="mt-5 text-sm font-semibold text-gray-900 dark:text-white">Body</h3>
        <pre class="mt-2 whitespace-pre-wrap rounded-md bg-gray-950 p-3 text-xs text-gray-100">{{ detail.body_excerpt || '-' }}</pre>
      </div>
    </div>

    <div v-if="snapshotRun" class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" @click.self="snapshotRun = null">
      <div class="max-h-[85vh] w-full max-w-3xl overflow-auto rounded-lg bg-white p-5 shadow-xl dark:bg-dark-900">
        <div class="flex items-center justify-between">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.codexInspection.detail.settingsSnapshot') }} #{{ snapshotRun.id }}</h2>
          <button class="icon-btn" @click="snapshotRun = null"><Icon name="x" size="sm" /></button>
        </div>
        <pre class="mt-4 whitespace-pre-wrap rounded-md bg-gray-950 p-3 text-xs text-gray-100">{{ formatJSON(snapshotRun.settings_snapshot) }}</pre>
      </div>
    </div>

    <div v-if="deleteTarget" class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" @click.self="deleteTarget = null">
      <div class="w-full max-w-md rounded-lg bg-white p-5 shadow-xl dark:bg-dark-900">
        <h2 class="text-lg font-semibold text-red-600">{{ t('admin.codexInspection.deleteConfirmTitle') }}</h2>
        <p class="mt-2 text-sm text-gray-600 dark:text-gray-300">{{ t('admin.codexInspection.deleteConfirmMessage') }}</p>
        <input v-model="deleteConfirmation" class="form-input mt-4" placeholder="DELETE" />
        <div class="mt-5 flex justify-end gap-2">
          <button class="btn-secondary" @click="deleteTarget = null">{{ t('common.cancel') }}</button>
          <button class="btn-danger" :disabled="deleteConfirmation !== 'DELETE'" @click="confirmDelete">{{ t('common.delete') }}</button>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import type { CodexInspectionAction, CodexInspectionLog, CodexInspectionOverview, CodexInspectionResult, CodexInspectionRun, CodexInspectionSettings } from '@/api/admin/codexInspection'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()

const activeTab = ref<'accounts' | 'runs' | 'logs' | 'settings'>('accounts')
const busy = ref(false)
const overviewData = ref<CodexInspectionOverview | null>(null)
const settingsDraft = ref<CodexInspectionSettings | null>(null)
const latestResults = ref<CodexInspectionResult[]>([])
const latestTotal = ref(0)
const latestPage = ref(1)
const latestPages = ref(1)
const runs = ref<CodexInspectionRun[]>([])
const logs = ref<CodexInspectionLog[]>([])
const selectedResults = ref<number[]>([])
const detail = ref<CodexInspectionResult | null>(null)
const snapshotRun = ref<CodexInspectionRun | null>(null)
const deleteTarget = ref<CodexInspectionResult | null>(null)
const deleteConfirmation = ref('')
const timePointsText = ref('')
const targetGroupIdsText = ref('')
const targetAccountIdsText = ref('')
const currentRunResultsId = ref<number | null>(null)
const moreMenuRowID = ref<number | null>(null)

const filters = ref({ search: '', action: '', probe_status: '', account_status: '', quota_window: '', group_ids_text: '', only_stale_minutes: 0 })
const logFilters = ref<{ run_id: number | ''; level: string }>({ run_id: '', level: '' })
let pollTimer: ReturnType<typeof setInterval> | null = null

const actionOptions: CodexInspectionAction[] = ['keep', 'enable', 'disable', 'reauth', 'delete']
const tabs = computed(() => [
  { key: 'accounts', label: t('admin.codexInspection.tabs.accounts') },
  { key: 'runs', label: t('admin.codexInspection.tabs.runs') },
  { key: 'logs', label: t('admin.codexInspection.tabs.logs') },
  { key: 'settings', label: t('admin.codexInspection.tabs.settings') },
] as const)

const runningRun = computed(() => overviewData.value?.running_run || null)
const progressPercent = computed(() => {
  const run = runningRun.value
  if (!run || run.total_accounts <= 0) return 0
  return Math.min(100, Math.round((run.completed_accounts / run.total_accounts) * 100))
})
const summaryCards = computed(() => {
  const o = overviewData.value
  return [
    { key: 'total', label: t('admin.codexInspection.summary.totalOauth'), value: o?.total_openai_oauth ?? '-' },
    { key: 'healthy', label: t('admin.codexInspection.summary.healthy'), value: o?.healthy_accounts ?? '-' },
    { key: 'five', label: t('admin.codexInspection.summary.fiveHourFull'), value: o?.five_hour_full_accounts ?? '-' },
    { key: 'long', label: t('admin.codexInspection.summary.longFull'), value: o?.long_window_full_accounts ?? '-' },
    { key: 'failed', label: t('admin.codexInspection.summary.failed'), value: o?.probe_failed_accounts ?? '-' },
    { key: 'reauth', label: t('admin.codexInspection.summary.reauth'), value: o?.reauth_accounts ?? '-' },
    { key: 'delete', label: t('admin.codexInspection.summary.delete'), value: o?.delete_suggested_accounts ?? '-' },
    { key: 'disabled', label: t('admin.codexInspection.summary.disabled'), value: o?.disabled_by_inspection_accounts ?? '-' },
    { key: 'latest', label: t('admin.codexInspection.summary.latest'), value: formatDate(o?.latest_run?.started_at || null) },
    { key: 'running', label: t('admin.codexInspection.summary.running'), value: runningRun.value ? `#${runningRun.value.id}` : '-' },
  ]
})
const allVisibleSelected = computed(() => latestResults.value.length > 0 && latestResults.value.every(row => selectedResults.value.includes(row.id)))

watch(settingsDraft, (value) => {
  timePointsText.value = value?.schedule.time_points?.join(',') || ''
  targetGroupIdsText.value = value?.target.group_ids?.join(',') || ''
  targetAccountIdsText.value = value?.target.account_ids?.join(',') || ''
}, { deep: true })

onMounted(async () => {
  await reload()
  pollTimer = setInterval(async () => {
    if (runningRun.value) await reload(false)
  }, 2000)
})

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
})

async function reload(showSpinner = true) {
  if (showSpinner) busy.value = true
  try {
    const [overview, settings] = await Promise.all([
      adminAPI.codexInspection.overview(),
      adminAPI.codexInspection.getSettings(),
    ])
    overviewData.value = overview
    settingsDraft.value = clone(settings)
    await Promise.all([loadLatest(), loadRuns(), loadLogs()])
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.codexInspection.loadError')))
  } finally {
    if (showSpinner) busy.value = false
  }
}

async function loadLatest() {
  const params = { ...accountResultQueryParams(), page: latestPage.value, page_size: 50 }
  const res = currentRunResultsId.value
    ? await adminAPI.codexInspection.listRunResults(currentRunResultsId.value, params)
    : await adminAPI.codexInspection.latestAccounts(params)
  latestResults.value = res.items || []
  latestTotal.value = res.total || 0
  latestPages.value = res.pages || 1
}

async function loadRuns() {
  const res = await adminAPI.codexInspection.listRuns({ limit: 50 })
  runs.value = res.items || []
}

async function loadLogs() {
  const res = await adminAPI.codexInspection.listLogs({
    limit: 100,
    run_id: logFilters.value.run_id || undefined,
    level: logFilters.value.level || undefined,
  })
  logs.value = res.items || []
}

async function startInspection() {
  busy.value = true
  try {
    await adminAPI.codexInspection.createRun({ apply_actions: false })
    await reload(false)
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.codexInspection.runError')))
  } finally {
    busy.value = false
  }
}

async function inspectSelected() {
  const accountIds = selectedAccountIDs()
  if (!accountIds.length) return
  busy.value = true
  try {
    await adminAPI.codexInspection.createRun({ account_ids: accountIds, apply_actions: false })
    selectedResults.value = []
    await reload(false)
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.codexInspection.runError')))
  } finally {
    busy.value = false
  }
}

async function cancelRunning() {
  if (!runningRun.value) return
  await adminAPI.codexInspection.cancelRun(runningRun.value.id)
  await reload()
}

async function probe(row: CodexInspectionResult) {
  busy.value = true
  try {
    await adminAPI.codexInspection.probeAccount(row.account_id)
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.codexInspection.probeError')))
  } finally {
    busy.value = false
    await reload(false)
  }
}

async function applyResult(row: CodexInspectionResult, actionOverride: '' | CodexInspectionAction = '') {
  const action = actionOverride || row.recommended_action
  if (action === 'delete') {
    openDelete(row)
    return
  }
  busy.value = true
  try {
    await adminAPI.codexInspection.applyActions(row.run_id, {
      result_ids: [row.id],
      action_override: actionOverride,
    })
    await reload(false)
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.codexInspection.actionError')))
  } finally {
    busy.value = false
  }
}

async function applySelected() {
  const rows = latestResults.value.filter(row => selectedResults.value.includes(row.id))
  const actionableRows = rows.filter(row => row.recommended_action !== 'delete')
  if (actionableRows.length !== rows.length) {
    appStore.showError(t('admin.codexInspection.deleteBulkSkipped'))
  }
  const byRun = new Map<number, number[]>()
  for (const row of actionableRows) {
    byRun.set(row.run_id, [...(byRun.get(row.run_id) || []), row.id])
  }
  busy.value = true
  try {
    for (const [runID, ids] of byRun) {
      await adminAPI.codexInspection.applyActions(runID, { result_ids: ids })
    }
    selectedResults.value = []
    await reload(false)
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.codexInspection.actionError')))
  } finally {
    busy.value = false
  }
}

function openDelete(row: CodexInspectionResult) {
  deleteTarget.value = row
  deleteConfirmation.value = ''
  moreMenuRowID.value = null
}

async function confirmDelete() {
  if (!deleteTarget.value) return
  busy.value = true
  try {
    await adminAPI.codexInspection.applyActions(deleteTarget.value.run_id, {
      result_ids: [deleteTarget.value.id],
      action_override: 'delete',
      confirmation_text: deleteConfirmation.value,
    })
    deleteTarget.value = null
    await reload(false)
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.codexInspection.actionError')))
  } finally {
    busy.value = false
  }
}

async function viewRun(run: CodexInspectionRun) {
  activeTab.value = 'accounts'
  resetAccountFilters()
  currentRunResultsId.value = run.id
  latestPage.value = 1
  await loadLatest()
}

async function rerunSame(run: CodexInspectionRun) {
  busy.value = true
  try {
    await adminAPI.codexInspection.createRun({
      apply_actions: false,
      settings_override: clone(run.settings_snapshot),
    })
    await reload()
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.codexInspection.runError')))
  } finally {
    busy.value = false
  }
}

async function rerunFailed(run: CodexInspectionRun) {
  const rows = await fetchAllRunResults(run.id, { probe_status: 'failed' })
  const accountIds = Array.from(new Set(rows.map(row => row.account_id)))
  if (!accountIds.length) {
    appStore.showSuccess(t('admin.codexInspection.noFailedAccounts'))
    return
  }
  await adminAPI.codexInspection.createRun({ account_ids: accountIds, apply_actions: false })
  await reload()
}

async function exportRun(run: CodexInspectionRun) {
  const rows = await fetchAllRunResults(run.id)
  exportRows(rows, `codex-inspection-run-${run.id}.csv`)
}

async function saveSettings() {
  if (!settingsDraft.value) return false
  syncTimePoints()
  syncTargetIds()
  busy.value = true
  try {
    const saved = await adminAPI.codexInspection.updateSettings(settingsDraft.value)
    settingsDraft.value = clone(saved)
    appStore.showSuccess(t('common.saved'))
    await reload(false)
    return true
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.codexInspection.saveError')))
    return false
  } finally {
    busy.value = false
  }
}

async function saveAndRun() {
  if (await saveSettings()) {
    await startInspection()
  }
}

function resetSettingsDraft() {
  settingsDraft.value = defaultSettings()
}

function syncTimePoints() {
  if (!settingsDraft.value) return
  settingsDraft.value.schedule.time_points = timePointsText.value.split(',').map(v => v.trim()).filter(Boolean)
}

function syncTargetIds() {
  if (!settingsDraft.value) return
  settingsDraft.value.target.group_ids = parseIDList(targetGroupIdsText.value)
  settingsDraft.value.target.account_ids = parseIDList(targetAccountIdsText.value)
}

function toggleAllVisible(event: Event) {
  const checked = (event.target as HTMLInputElement).checked
  const visible = latestResults.value.map(row => row.id)
  selectedResults.value = checked
    ? Array.from(new Set([...selectedResults.value, ...visible]))
    : selectedResults.value.filter(id => !visible.includes(id))
}

function changeLatestPage(delta: number) {
  latestPage.value = Math.min(latestPages.value, Math.max(1, latestPage.value + delta))
  loadLatest()
}

async function exportCSV() {
  if (currentRunResultsId.value) {
    const rows = await fetchAllRunResults(currentRunResultsId.value, accountResultQueryParams())
    exportRows(rows, `codex-inspection-run-${currentRunResultsId.value}-${Date.now()}.csv`)
    return
  }
  exportRows(latestResults.value, `codex-inspection-${Date.now()}.csv`)
}

function exportRows(rowsToExport: CodexInspectionResult[], filename: string) {
  const header = ['account_id', 'account_name', 'chatgpt_account_id', 'proxy_id_snapshot', 'probe_status', 'upstream_status_code', 'latency_ms', 'five_hour_used_percent', 'long_window_type', 'long_window_used_percent', 'recommended_action', 'action_status', 'action_error', 'action_reason', 'created_at']
  const rows = rowsToExport.map(row => header.map(key => JSON.stringify((row as unknown as Record<string, unknown>)[key] ?? '')).join(','))
  const blob = new Blob([[header.join(','), ...rows].join('\n')], { type: 'text/csv;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  link.click()
  URL.revokeObjectURL(url)
}

function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value))
}

function parseIDList(value: string) {
  return Array.from(new Set(value.split(',').map(v => Number(v.trim())).filter(v => Number.isInteger(v) && v > 0)))
}

function openAccount(row: CodexInspectionResult) {
  router.push({ path: '/admin/accounts', query: { search: row.account_name || String(row.account_id) } })
}

function accountResultQueryParams() {
  const groupIDs = parseIDList(filters.value.group_ids_text)
  return {
    search: filters.value.search || undefined,
    action: filters.value.action || undefined,
    probe_status: filters.value.probe_status || undefined,
    account_status: filters.value.account_status || undefined,
    quota_window: filters.value.quota_window || undefined,
    group_ids: groupIDs.length ? groupIDs.join(',') : undefined,
    only_stale_minutes: filters.value.only_stale_minutes || undefined,
  }
}

async function fetchAllRunResults(runID: number, extraParams: Record<string, unknown> = {}) {
  const pageSize = 200
  const rows: CodexInspectionResult[] = []
  for (let page = 1; ; page += 1) {
    const res = await adminAPI.codexInspection.listRunResults(runID, { ...extraParams, page, page_size: pageSize })
    rows.push(...(res.items || []))
    if (rows.length >= (res.total || 0) || (res.items || []).length < pageSize) break
  }
  return rows
}

function selectedAccountIDs() {
  const ids = latestResults.value
    .filter(row => selectedResults.value.includes(row.id))
    .map(row => row.account_id)
  return Array.from(new Set(ids))
}

function clearRunResults() {
  currentRunResultsId.value = null
  latestPage.value = 1
  selectedResults.value = []
  resetAccountFilters()
  loadLatest()
}

function resetAccountFilters() {
  filters.value = { search: '', action: '', probe_status: '', account_status: '', quota_window: '', group_ids_text: '', only_stale_minutes: 0 }
}

function toggleMore(rowID: number) {
  moreMenuRowID.value = moreMenuRowID.value === rowID ? null : rowID
}

function defaultSettings(): CodexInspectionSettings {
  return {
    enabled: false,
    schedule: {
      mode: 'interval',
      interval_minutes: 60,
      time_points: [],
      timezone: 'Asia/Shanghai',
    },
    target: {
      only_openai_oauth: true,
      account_ids: [],
      group_ids: [],
      include_unschedulable: true,
      include_error: false,
      only_stale_minutes: 0,
      sample_size: 0,
    },
    probe: {
      workers: 4,
      timeout_ms: 15000,
      retries: 0,
      min_interval_minutes: 30,
      user_agent: '',
    },
    decision: {
      used_percent_threshold: 100,
      short_window_policy: 'keep',
      long_window_policy: 'disable',
    },
    actions: {
      auto_apply: false,
      allow_enable: false,
      allow_disable: false,
      allow_mark_reauth: false,
      allow_delete: false,
    },
  }
}

function formatJSON(value: unknown) {
  try {
    return JSON.stringify(value || {}, null, 2)
  } catch {
    return '{}'
  }
}

function percent(value: number | null) {
  if (value === null || value === undefined) return '-'
  return `${value.toFixed(1)}%`
}

function longWindowText(row: CodexInspectionResult) {
  if (row.long_window_type === 'none') return '-'
  return `${row.long_window_type} ${percent(row.long_window_used_percent)}`
}

function formatDate(value?: string | null) {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

function runDuration(run: CodexInspectionRun) {
  if (!run.started_at || !run.finished_at) return '-'
  const ms = new Date(run.finished_at).getTime() - new Date(run.started_at).getTime()
  if (!Number.isFinite(ms) || ms < 0) return '-'
  const seconds = Math.round(ms / 1000)
  if (seconds < 60) return `${seconds}s`
  return `${Math.floor(seconds / 60)}m ${seconds % 60}s`
}

function actionLabel(action: string) {
  return t(`admin.codexInspection.action.${action}`)
}

function actionStatusLabel(status: string) {
  return t(`admin.codexInspection.actionStatus.${status}`)
}

function probeLabel(status: string) {
  return t(`admin.codexInspection.probe.${status}`)
}

function actionClass(action: string) {
  return {
    keep: 'badge-gray',
    enable: 'badge-green',
    disable: 'badge-amber',
    reauth: 'badge-blue',
    delete: 'badge-red',
  }[action] || 'badge-gray'
}

function probeClass(status: string) {
  return status === 'success' ? 'badge-green' : status === 'failed' ? 'badge-red' : 'badge-gray'
}

function actionStatusClass(status: string) {
  return status === 'success' ? 'badge-green' : status === 'failed' ? 'badge-red' : status === 'needs_review' ? 'badge-amber' : status === 'skipped' ? 'badge-gray' : 'badge-blue'
}

function runClass(status: string) {
  return status === 'running' ? 'badge-blue' : status === 'completed' ? 'badge-green' : status === 'failed' ? 'badge-red' : 'badge-gray'
}
</script>

<style scoped>
.btn-primary,
.btn-secondary,
.btn-danger {
  @apply inline-flex items-center gap-1.5 rounded-md px-3 py-2 text-sm font-medium transition disabled:cursor-not-allowed disabled:opacity-50;
}
.btn-primary {
  @apply bg-blue-600 text-white hover:bg-blue-700;
}
.btn-secondary {
  @apply border border-gray-200 bg-white text-gray-700 hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-200 dark:hover:bg-dark-800;
}
.btn-danger {
  @apply bg-red-600 text-white hover:bg-red-700;
}
.icon-btn {
  @apply inline-flex h-8 w-8 items-center justify-center rounded-md border border-gray-200 bg-white text-gray-600 hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-300 dark:hover:bg-dark-800;
}
.icon-btn.danger {
  @apply text-red-600 hover:border-red-200 hover:bg-red-50 dark:hover:bg-red-950/30;
}
.more-menu {
  @apply absolute right-0 top-9 z-20 min-w-32 rounded-md border border-gray-200 bg-white p-1 shadow-lg dark:border-dark-700 dark:bg-dark-900;
}
.more-menu-item {
  @apply flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm text-gray-700 hover:bg-gray-50 dark:text-gray-200 dark:hover:bg-dark-800;
}
.more-menu-item.danger {
  @apply text-red-600 hover:bg-red-50 dark:hover:bg-red-950/30;
}
.tab-button {
  @apply border-b-2 border-transparent px-4 py-3 text-sm font-medium text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white;
}
.tab-button-active {
  @apply border-blue-600 text-blue-600 dark:text-blue-400;
}
.form-input {
  @apply rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500 dark:border-dark-700 dark:bg-dark-800 dark:text-white;
}
.badge {
  @apply inline-flex w-fit items-center rounded-md px-2 py-0.5 text-xs font-medium;
}
.badge-gray {
  @apply bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200;
}
.badge-green {
  @apply bg-green-100 text-green-700 dark:bg-green-950/50 dark:text-green-300;
}
.badge-amber {
  @apply bg-amber-100 text-amber-800 dark:bg-amber-950/50 dark:text-amber-300;
}
.badge-blue {
  @apply bg-blue-100 text-blue-700 dark:bg-blue-950/50 dark:text-blue-300;
}
.badge-red {
  @apply bg-red-100 text-red-700 dark:bg-red-950/50 dark:text-red-300;
}
.settings-panel {
  @apply rounded-lg border border-gray-200 p-4 dark:border-dark-700;
}
.settings-panel h2 {
  @apply mb-4 text-sm font-semibold text-gray-900 dark:text-white;
}
.field-label {
  @apply mt-3 block text-xs font-medium text-gray-500 dark:text-gray-400;
}
.toggle-row {
  @apply mt-3 flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200;
}
.detail-box {
  @apply rounded-md border border-gray-200 p-3 text-sm text-gray-700 dark:border-dark-700 dark:text-gray-300;
}
</style>
