import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { Pill } from "./pill";

describe("Pill", () => {
  it("renders the dot inside the child element with asChild", () => {
    const html = renderToStaticMarkup(
      <Pill asChild color="#20975C">
        <button type="button">Groceries</button>
      </Pill>
    );
    expect(html.startsWith("<button")).toBe(true);
    expect(html).toContain('data-slot="pill"');
    expect(html).toContain("background-color:#20975C");
    expect(html).toContain("Groceries</button>");
  });

  it("renders a span without asChild", () => {
    const html = renderToStaticMarkup(<Pill color="#DB5757">Dining</Pill>);
    expect(html.startsWith("<span")).toBe(true);
    expect(html).toContain("Dining");
  });
});
