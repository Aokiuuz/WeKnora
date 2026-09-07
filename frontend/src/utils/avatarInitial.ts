/** Derive a readable avatar label without changing stored names or avatars. */
export function avatarInitial(name: string, fallback = '?'): string {
  return name.match(/[\p{L}\p{N}]/u)?.[0]?.toLocaleUpperCase() || fallback
}
