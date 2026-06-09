import { describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'

import AccountTableFilters from '../AccountTableFilters.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const SelectStub = defineComponent({
  name: 'SelectFieldStub',
  props: {
    modelValue: {
      type: [String, Number, Boolean, null],
      default: ''
    },
    options: {
      type: Array,
      default: () => []
    }
  },
  emits: ['update:modelValue', 'change'],
  template: `
    <select
      :value="modelValue"
      @change="onChange"
    >
      <option v-for="option in options" :key="option.value" :value="option.value">
        {{ option.label }}
      </option>
    </select>
  `,
  methods: {
    onChange(event: Event) {
      const target = event.target as HTMLSelectElement
      this.$emit('update:modelValue', target.value)
      this.$emit('change')
    }
  }
})

const SearchInputStub = defineComponent({
  name: 'SearchInput',
  props: {
    modelValue: {
      type: String,
      default: ''
    }
  },
  emits: ['update:modelValue', 'search'],
  template: '<input :value="modelValue" @input="onInput" />',
  methods: {
    onInput(event: Event) {
      const target = event.target as HTMLInputElement
      this.$emit('update:modelValue', target.value)
    }
  }
})

describe('AccountTableFilters', () => {
  it('emits OpenAI plan_type filter updates separately from auth type', async () => {
    const wrapper = mount(AccountTableFilters, {
      props: {
        searchQuery: '',
        filters: {
          platform: '',
          type: '',
          plan_type: '',
          status: '',
          privacy_mode: '',
          group: '',
          search: ''
        },
        groups: []
      },
      global: {
        stubs: {
          Select: SelectStub,
          SearchInput: SearchInputStub
        }
      }
    })

    const selects = wrapper.findAll('select')
    expect(selects).toHaveLength(6)
    expect(selects[2].findAll('option').map(option => option.attributes('value'))).toEqual([
      '',
      'free',
      'plus',
      'team',
      'pro'
    ])

    await selects[2].setValue('pro')

    expect(wrapper.emitted('update:filters')?.at(-1)?.[0]).toMatchObject({
      type: '',
      plan_type: 'pro'
    })
  })
})
