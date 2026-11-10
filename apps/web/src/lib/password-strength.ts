export interface PasswordStrength {
  /** Filled segments out of 4. */
  level: 0 | 1 | 2 | 3 | 4;
  label: string;
}

/**
 * Length-only strength meter: long passphrases beat short "complex" ones, and
 * the server only enforces 8 to 128 characters. Nothing is sent anywhere.
 */
export function passwordStrength(password: string): PasswordStrength {
  const n = password.length;
  if (n === 0) return { level: 0, label: "At least 8 characters" };
  if (n < 8) return { level: 1, label: `Too short · ${n} of 8 characters` };
  if (n < 12) return { level: 2, label: `Okay · ${n} characters` };
  if (n < 16) return { level: 3, label: `Good · ${n} characters` };
  return { level: 4, label: `Strong · ${n} characters` };
}
