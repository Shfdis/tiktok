import type { Move, MyState, State } from "./types";

async function readJson<T>(res: Response): Promise<T> {
  const text = await res.text();
  if (!res.ok) {
    try {
      const parsed = JSON.parse(text) as { error?: string };
      throw new Error(parsed?.error || res.statusText);
    } catch {
      throw new Error(text || res.statusText);
    }
  }
  return JSON.parse(text) as T;
}

export async function findMatch(signal?: AbortSignal): Promise<MyState> {
  const res = await fetch("/play", { method: "POST", signal });
  return readJson<MyState>(res);
}

export async function getState(id: number, signal?: AbortSignal): Promise<MyState> {
  const res = await fetch(`/play?id=${encodeURIComponent(String(id))}`, { method: "GET", signal });
  return readJson<MyState>(res);
}

export async function makeMove(id: number, move: Move): Promise<State> {
  const res = await fetch(`/play?id=${encodeURIComponent(String(id))}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(move),
  });
  return readJson<State>(res);
}


