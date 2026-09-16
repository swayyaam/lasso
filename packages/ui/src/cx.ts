/**
 * cx joins class names, dropping anything falsy.
 *
 * A three-line helper rather than a dependency: the app needs conditional
 * classes, not a class-name DSL.
 */
export function cx(...parts: Array<string | false | null | undefined>): string {
  return parts.filter(Boolean).join(" ");
}
