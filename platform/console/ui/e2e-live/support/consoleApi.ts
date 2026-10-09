// The console's own write API, bearer-authenticated with the UI token —
// used only to ARRANGE a fixture's starting state (closing a conversation so
// a test can assert the closed/show-closed behaviour against a conversation
// that is ALREADY closed, logging in to mint the session cookie the browser
// reuses). Every assertion in this suite reads the rendered UI instead.

export class ConsoleApiClient {
  constructor(
    private readonly baseUrl: string,
    private readonly uiToken: string,
  ) {}

  private async request(method: string, path: string, body?: unknown): Promise<{ status: number; json: unknown }> {
    const res = await fetch(this.baseUrl + path, {
      method,
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${this.uiToken}` },
      body: body ? JSON.stringify(body) : undefined,
    })
    const text = await res.text()
    let json: unknown
    try {
      json = text ? JSON.parse(text) : undefined
    } catch {
      json = text
    }
    return { status: res.status, json }
  }

  async get(path: string): Promise<unknown> {
    const { status, json } = await this.request('GET', path)
    if (status !== 200) throw new Error(`GET ${path}: ${status} ${JSON.stringify(json)}`)
    return json
  }

  async closeConversations(names: string[]): Promise<void> {
    const { status, json } = await this.request('POST', '/api/conversations/close', { names })
    if (status !== 200) throw new Error(`close ${names.join(',')}: ${status} ${JSON.stringify(json)}`)
  }

  async sendMessage(name: string, text: string): Promise<void> {
    const { status, json } = await this.request('POST', `/api/conversations/${name}/messages`, { text })
    if (Math.floor(status / 100) !== 2) throw new Error(`send to ${name}: ${status} ${JSON.stringify(json)}`)
  }
}
