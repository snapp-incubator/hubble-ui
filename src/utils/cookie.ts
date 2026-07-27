export const TOKEN = 'token';

export interface JwtPayload {
  username: string;
  iat: number;
  exp: number;
}

export function getAuthToken(): string | null {
  return getCookie(TOKEN);
}

export function getAuthClaims(token: string): JwtPayload | null {
  try {
    const parts = token.split('.');
    if (parts.length !== 3) return null;
    const payload = JSON.parse(atob(parts[1].replace(/-/g, '+').replace(/_/g, '/')));
    if (!payload || typeof payload.username !== 'string') return null;
    return payload as JwtPayload;
  } catch {
    return null;
  }
}

export function getCookie(name: string): string | null {
  const nameLenPlus = name.length + 1;
  return (
    document.cookie
      .split(';')
      .map(c => c.trim())
      .filter(cookie => cookie.substring(0, nameLenPlus) === `${name}=`)
      .map(cookie => decodeURIComponent(cookie.substring(nameLenPlus)))[0] || null
  );
}

export function deleteAuthCookie() {
  document.cookie = `${TOKEN}=; Path=/; Max-Age=0; Expires=Thu, 01 Jan 1970 00:00:00 GMT`;
}
