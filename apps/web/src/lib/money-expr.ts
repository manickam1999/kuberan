/**
 * Pure evaluator for arithmetic typed into money fields (e.g. "12.50+3*2").
 * Exact rational arithmetic (BigInt), rounded once to cents at the end.
 * No eval / new Function: safe under a strict CSP.
 */

/** Result of evaluating the text of a money field. */
export type AmountEvaluation =
  | { ok: true; cents: number; isExpression: boolean }
  | { ok: false; reason: "empty" | "incomplete" | "invalid" | "div_zero" };

/** Characters a money field accepts while typing. */
export const AMOUNT_INPUT_PATTERN = /^[\d.,+\-*/×÷xX()\s−]*$/;

// BigInt literals need target >= ES2020; the tsconfig targets lower.
const ZERO = BigInt(0);
const ONE = BigInt(1);
const TWO = BigInt(2);
const TEN = BigInt(10);
const HUNDRED = BigInt(100);

function pow10(k: number): bigint {
  let r = ONE;
  for (let i = 0; i < k; i++) r *= TEN;
  return r;
}

interface Rat {
  n: bigint;
  d: bigint;
}

type Token =
  | { t: "num"; v: Rat }
  | { t: "op"; v: "+" | "-" | "*" | "/" | "(" | ")" };

class ParseError extends Error {
  constructor(readonly reason: "incomplete" | "invalid" | "div_zero") {
    super(reason);
  }
}

function gcd(a: bigint, b: bigint): bigint {
  let x = a < ZERO ? -a : a;
  let y = b < ZERO ? -b : b;
  while (y !== ZERO) {
    [x, y] = [y, x % y];
  }
  return x;
}

function rat(n: bigint, d: bigint): Rat {
  if (d < ZERO) {
    n = -n;
    d = -d;
  }
  const g = gcd(n, d);
  return g > ONE ? { n: n / g, d: d / g } : { n, d };
}

function tokenize(s: string): Token[] {
  const tokens: Token[] = [];
  let i = 0;
  while (i < s.length) {
    const c = s[i];
    if ("+-*/()".includes(c)) {
      tokens.push({ t: "op", v: c as "+" | "-" | "*" | "/" | "(" | ")" });
      i++;
    } else if ((c >= "0" && c <= "9") || c === ".") {
      let j = i;
      while (j < s.length && /[\d.,]/.test(s[j])) j++;
      let text = s.slice(i, j);
      if (text.endsWith(",")) {
        // A trailing comma at end of input may still become a thousands group.
        const rest = text.slice(0, -1);
        if (j >= s.length && /^\d{1,3}(,\d{3})*$/.test(rest)) {
          throw new ParseError("incomplete");
        }
        throw new ParseError("invalid");
      }
      if (text.includes(",") && !/^\d{1,3}(,\d{3})+(\.\d*)?$/.test(text)) {
        throw new ParseError("invalid");
      }
      text = text.replace(/,/g, "");
      const dots = text.split(".").length - 1;
      const digits = text.length - dots;
      if (dots > 1) throw new ParseError("invalid");
      if (digits === 0) {
        // lone "."
        if (j >= s.length) throw new ParseError("incomplete");
        throw new ParseError("invalid");
      }
      const dot = text.indexOf(".");
      if (dot === -1) {
        tokens.push({ t: "num", v: { n: BigInt(text), d: ONE } });
      } else {
        const frac = text.slice(dot + 1);
        const int = text.slice(0, dot) + frac;
        tokens.push({
          t: "num",
          v: rat(BigInt(int === "" ? "0" : int), pow10(frac.length)),
        });
      }
      i = j;
    } else {
      throw new ParseError("invalid");
    }
  }
  return tokens;
}

class Parser {
  private pos = 0;
  constructor(private readonly tokens: Token[]) {}

  parseAll(): Rat {
    const v = this.expr();
    if (this.pos < this.tokens.length) throw new ParseError("invalid");
    return v;
  }

  private peekOp(): string | null {
    const tok = this.tokens[this.pos];
    return tok && tok.t === "op" ? tok.v : null;
  }

  private expr(): Rat {
    let left = this.term();
    for (;;) {
      const op = this.peekOp();
      if (op !== "+" && op !== "-") return left;
      this.pos++;
      const right = this.term();
      left =
        op === "+"
          ? rat(left.n * right.d + right.n * left.d, left.d * right.d)
          : rat(left.n * right.d - right.n * left.d, left.d * right.d);
    }
  }

  private term(): Rat {
    let left = this.factor();
    for (;;) {
      const op = this.peekOp();
      if (op !== "*" && op !== "/") return left;
      this.pos++;
      const right = this.factor();
      if (op === "*") {
        left = rat(left.n * right.n, left.d * right.d);
      } else {
        if (right.n === ZERO) throw new ParseError("div_zero");
        left = rat(left.n * right.d, left.d * right.n);
      }
    }
  }

  private factor(): Rat {
    const tok = this.tokens[this.pos];
    if (!tok) throw new ParseError("incomplete");
    if (tok.t === "num") {
      this.pos++;
      return tok.v;
    }
    if (tok.v === "-" || tok.v === "+") {
      this.pos++;
      const v = this.factor();
      return tok.v === "-" ? { n: -v.n, d: v.d } : v;
    }
    if (tok.v === "(") {
      this.pos++;
      const v = this.expr();
      const close = this.tokens[this.pos];
      if (!close) throw new ParseError("incomplete");
      if (close.t !== "op" || close.v !== ")") throw new ParseError("invalid");
      this.pos++;
      return v;
    }
    throw new ParseError("invalid");
  }
}

/** Divide and round half away from zero (d > 0). */
function roundRat(r: Rat): bigint {
  const neg = r.n < ZERO;
  const abs = neg ? -r.n : r.n;
  const q = (TWO * abs + r.d) / (TWO * r.d);
  return neg ? -q : q;
}

/**
 * Evaluates money-field text (a plain number or an arithmetic expression, in
 * major units such as ringgit) and returns the result in integer cents,
 * rounded half away from zero. `incomplete` means the text is a valid prefix
 * that ended early (e.g. "12+").
 */
export function evaluateAmount(text: string): AmountEvaluation {
  const s = text
    .replace(/\s/g, "")
    .replace(/[xX×]/g, "*")
    .replace(/÷/g, "/")
    .replace(/−/g, "-");
  if (s === "") return { ok: false, reason: "empty" };

  try {
    const tokens = tokenize(s);
    const value = new Parser(tokens).parseAll();
    const centsBig = roundRat({ n: value.n * HUNDRED, d: value.d });
    const limit = BigInt(Number.MAX_SAFE_INTEGER);
    if (centsBig > limit || centsBig < -limit) {
      return { ok: false, reason: "invalid" };
    }
    // Plain number, optionally with a leading sign, is not an expression.
    const isExpression = !(
      tokens.length === 1 ||
      (tokens.length === 2 &&
        tokens[0].t === "op" &&
        (tokens[0].v === "-" || tokens[0].v === "+") &&
        tokens[1].t === "num")
    );
    return { ok: true, cents: Number(centsBig), isExpression };
  } catch (e) {
    if (e instanceof ParseError) return { ok: false, reason: e.reason };
    throw e;
  }
}

/**
 * Formats integer cents as a plain decimal string without symbol or grouping,
 * using integer math: 2140 -> "21.40", -350 -> "-3.50".
 */
export function formatCentsPlain(cents: number): string {
  const neg = cents < 0;
  const abs = Math.abs(cents);
  const whole = Math.floor(abs / 100);
  const frac = abs % 100;
  return `${neg ? "-" : ""}${whole}.${frac < 10 ? "0" : ""}${frac}`;
}

/**
 * Evaluate like evaluateAmount, but finish an incomplete expression the way a
 * calculator would: drop trailing operators/dots and close open parentheses
 * ("12+" -> 12, "(12+3" -> 15, "(4*(2+1" -> 12). Non-incomplete results are
 * returned unchanged.
 */
export function evaluateAmountLenient(text: string): AmountEvaluation {
  const first = evaluateAmount(text);
  if (first.ok || first.reason !== "incomplete") return first;
  let t = text
    .replace(/\s/g, "")
    .replace(/[,.+\-*/×÷xX\u2212(]+$/, "");
  const open = t.split("(").length - t.split(")").length;
  if (open > 0) t += ")".repeat(open);
  const done = evaluateAmount(t);
  return done.ok ? done : first;
}
