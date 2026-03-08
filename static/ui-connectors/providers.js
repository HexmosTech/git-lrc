export const providers = [
  {
    id: 'gemini',
    name: 'Google Gemini',
    defaultModel: 'gemini-2.5-flash',
    models: ['gemini-2.5-flash', 'gemini-2.5-flash-lite', 'gemini-2.5-pro', 'gemini-2.0-flash', 'gemini-2.0-flash-lite'],
    apiKeyPlaceholder: 'gemini-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
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
    defaultModel: 'gpt-4',
    models: ['gpt-3.5-turbo', 'gpt-4', 'gpt-4-turbo', 'gpt-4o'],
    apiKeyPlaceholder: 'sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
  },
  {
    id: 'claude',
    name: 'Anthropic Claude',
    defaultModel: 'claude-3-sonnet-20240229',
    models: ['claude-3-opus-20240229', 'claude-3-sonnet-20240229', 'claude-3-haiku-20240307'],
    apiKeyPlaceholder: 'claude-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
  },
  {
    id: 'cohere',
    name: 'Cohere',
    defaultModel: 'command-r',
    models: ['command', 'command-light', 'command-r', 'command-r-plus'],
    apiKeyPlaceholder: 'cohere-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
  },
];

export function defaultForm() {
  const first = providers[0];
  return {
    id: '',
    provider_name: first.id,
    connector_name: `${first.name} Connector`,
    api_key: '',
    base_url: '',
    selected_model: first.defaultModel,
  };
}
