// SimpleSpinner.ts
export class SimpleSpinner {
  private intervalId: number | null = null;
  private position: number = 0;
  private chars: string[] = ['|', '/', '-', '\\'];
  private message: string = '';
  private active: boolean = false;

  constructor(message: string = '') {
    this.message = message;
  }

  start(): void {
    if (this.active) return;
    this.active = true;
    this._render();
    this.intervalId = setInterval(() => {
      this._render();
    }, 100);
  }

  stop(): void {
    if (!this.active) return;
    this.active = false;
    if (this.intervalId) {
      clearInterval(this.intervalId);
      this.intervalId = null;
    }
    this._clearLine();
  }

  setMessage(message: string): void {
    this.message = message;
    this._render();
  }

  isActive(): boolean {
    return this.active;
  }

  private _render(): void {
    if (!this.active) return;
    this.position = (this.position + 1) % this.chars.length;
    const text = `\r\u001B[K${this.chars[this.position]} ${this.message}`;
    Deno.stdout.writeSync(new TextEncoder().encode(text));
  }

  private _clearLine(): void {
    Deno.stdout.writeSync(new TextEncoder().encode('\r\u001B[K'));
  }
}
