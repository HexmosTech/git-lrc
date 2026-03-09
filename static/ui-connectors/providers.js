import { generateFriendlyConnectorName } from '/static/ui-connectors/name-utils.js';

export const providers = [
  {
    id: 'gemini',
    name: 'Google Gemini',
    defaultModel: 'gemini-2.5-flash',
    models: ['gemini-2.5-flash', 'gemini-2.5-flash-lite', 'gemini-2.5-pro', 'gemini-2.0-flash', 'gemini-2.0-flash-lite'],
    apiKeyPlaceholder: 'gemini-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
  },
  {
    id: 'deepseek',
    name: 'DeepSeek',
    defaultModel: 'deepseek-chat',
    models: ['deepseek-chat', 'deepseek-r1'],
    apiKeyPlaceholder: 'sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
    baseURLPlaceholder: 'https://api.deepseek.com/v1',
  },
  {
    id: 'openrouter',
    name: 'OpenRouter',
    defaultModel: 'deepseek/deepseek-r1-0528:free',
    models: ['deepseek/deepseek-r1-0528:free'],
    apiKeyPlaceholder: 'sk-or-v1-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
    baseURLPlaceholder: 'https://openrouter.ai/api/v1',
  },
  {
    id: 'ollama',
    name: 'Ollama',
    defaultModel: '',
    models: [],
    requiresBaseURL: true,
    baseURLPlaceholder: 'http://localhost:11434/ollama/api',
    apiKeyPlaceholder: 'Optional JWT token for authentication',
  },
  {
    id: 'openai',
    name: 'OpenAI',
    defaultModel: 'o4-mini',
    models: ['o4-mini', 'gpt-4.1', 'gpt-4.1-mini', 'gpt-4o-mini', 'gpt-4o', 'o3-mini'],
    apiKeyPlaceholder: 'sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
  },
  {
    id: 'claude',
    name: 'Anthropic Claude',
    defaultModel: 'claude-haiku-4-5-20251001',
    models: [
      'claude-haiku-4-5-20251001',
      'claude-opus-4-1-20250805',
      'claude-opus-4-20250514',
      'claude-opus-4-5-20251101',
      'claude-opus-4-6',
      'claude-sonnet-4-20250514',
      'claude-sonnet-4-5-20250929',
      'claude-sonnet-4-6',
      'claude-3-opus-20240229',
      'claude-3-sonnet-20240229',
      'claude-3-haiku-20240307',
    ],
    apiKeyPlaceholder: 'sk-ant-api03-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
  },
];

export function defaultForm() {
  const first = providers[0];
  return {
    id: '',
    provider_name: first.id,
    connector_name: generateFriendlyConnectorName(first.id, providers),
    api_key: '',
    base_url: '',
    selected_model: first.defaultModel,
  };
}
