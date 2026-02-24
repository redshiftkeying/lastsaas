import axios from 'axios';

export function getErrorMessage(err: unknown): string {
  if (axios.isAxiosError(err)) {
    const data = err.response?.data;
    if (typeof data === 'string') return data;
    if (data && typeof data === 'object' && 'error' in data) return String(data.error);
    if (err.response?.status === 429) return 'Too many requests. Please try again later.';
    if (err.message) return err.message;
  }
  if (err instanceof Error) return err.message;
  return 'An unexpected error occurred';
}
