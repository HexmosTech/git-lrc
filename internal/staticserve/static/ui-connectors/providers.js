import { generateFriendlyConnectorName } from '/static/ui-connectors/name-utils.js';

export const providers = [
  {
    id: 'gemini',
    name: 'Google Gemini',
    apiKeyPlaceholder: 'gemini-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
  },
  {
    id: 'deepseek',
    name: 'DeepSeek',
    apiKeyPlaceholder: 'sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
    baseURLPlaceholder: 'https://api.deepseek.com/v1',
  },
  {
    id: 'openrouter',
    name: 'OpenRouter',
    apiKeyPlaceholder: 'sk-or-v1-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
    baseURLPlaceholder: 'https://openrouter.ai/api/v1',
  },
  {
    id: 'ollama',
    name: 'Ollama',
    requiresBaseURL: true,
    baseURLPlaceholder: 'http://localhost:11434/ollama/api',
    apiKeyPlaceholder: 'Optional JWT token for authentication',
  },
  {
    id: 'openai',
    name: 'OpenAI',
    apiKeyPlaceholder: 'sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
  },
  {
    id: 'claude',
    name: 'Anthropic Claude',
    apiKeyPlaceholder: 'sk-ant-api03-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
  },
  {
    id: 'anthropic-compatible',
    name: 'Anthropic Compatible API',
    requiresBaseURL: true,
    baseURLPresets: [
      {
        label: 'ClaudeAPI (gw.claudeapi.com)',
        value: 'https://gw.claudeapi.com/v1',
        models: ['claude-haiku-4-5-20251001', 'claude-sonnet-4-6', 'claude-opus-4-8'],
      },
    ],
    baseURLPlaceholder: 'https://your-anthropic-compatible-endpoint.com',
    apiKeyPlaceholder: 'sk-ant-api03-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
    modelPresets: [
      'claude-haiku-4-5-20251001',
      'claude-sonnet-4-6',
      'claude-opus-4-8',
      'claude-3-5-sonnet-20241022',
      'claude-3-5-haiku-20241022',
      'claude-3-opus-20240229',
      'claude-3-haiku-20240307',
    ]
  },
  {
    id: 'atlas',
    name: 'Atlas Cloud',
    apiKeyPlaceholder: 'sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
    baseURLPlaceholder: 'https://api.atlascloud.ai/v1',
  }
];

export function defaultForm() {
  const first = providers[0];
  return {
    id: '',
    provider_name: first.id,
    connector_name: generateFriendlyConnectorName(first.id, providers),
    api_key: '',
    base_url: '',
    selected_model: '',
  };
}
