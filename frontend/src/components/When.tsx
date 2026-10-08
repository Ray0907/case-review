import { relativeTime, reviewTime } from "../format";

// Recent times read as "3 hours ago"; the exact time stays on hover and in dateTime.
export default function When({ seconds }: { seconds: number }) {
  return <time className="tabular" dateTime={new Date(seconds * 1000).toISOString()} title={reviewTime(seconds)}>{relativeTime(seconds)}</time>;
}
