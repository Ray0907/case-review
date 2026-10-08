import CheckIcon from "./CheckIcon";

export type Step = { label: string; meta: string; done: boolean };

export default function Progress({ steps }: { steps: Step[] }) {
  // A step only counts as done once every step before it is done.
  const active = steps.findIndex((s) => !s.done);
  const done_at = (i: number) => steps[i].done && (active === -1 || i < active);
  return (
    <ol className="steps" role="list" aria-label="Review progress">
      {steps.map((s, i) => (
        <li key={s.label} className={`step${done_at(i) ? " step-done" : ""}${i === active ? " step-active" : ""}`}
          aria-current={i === active ? "step" : undefined}>
          <span className="step-index" aria-hidden="true">
            {done_at(i)
              ? <CheckIcon />
              : i + 1}
          </span>
          <span className="step-text">
            <span className="step-label">{s.label}</span>
            <span className="step-meta tabular">{s.meta}</span>
          </span>
        </li>
      ))}
    </ol>
  );
}
