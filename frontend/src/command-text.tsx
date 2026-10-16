import { Disclosure } from "@/renderer/components/ui/disclosure";
import type { Messages } from "./messages";

export function CommandText({ text, m }: { text: string; m: Pick<Messages, "commandText"> }) {
  const code = <code className="block whitespace-pre-wrap break-all text-xs leading-6">{text}</code>;
  if (text.length <= 180 && text.split("\n").length <= 3) return code;
  return <Disclosure label={m.commandText}
    summary={<code className="line-clamp-3 whitespace-pre-wrap break-all text-xs leading-6">{text}</code>}>
    {code}
  </Disclosure>;
}
