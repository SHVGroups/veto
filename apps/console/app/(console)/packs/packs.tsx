"use client";

import type { Route } from "next";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { Icon } from "@/components/icons";
import { useCan } from "@/components/shell/session";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Field, Input } from "@/components/ui/form";
import { PageHeader, Panel } from "@/components/ui/layout";
import { Pill } from "@/components/ui/pills";
import { ErrorState, LoadingPanel } from "@/components/ui/states";
import { LinkRow, RowLink, Table, Td, Th } from "@/components/ui/table";
import { usePacks } from "@/lib/queries";

const SLUG = /^[a-z][a-z0-9_]{0,62}$/;

export function PacksPage() {
  const q = usePacks();
  const router = useRouter();
  const canConfigure = useCan("configure");
  const [adding, setAdding] = useState(false);
  const [name, setName] = useState("");
  const taken = q.data?.items.some((p) => p.name === name);

  return (
    <>
      <PageHeader
        title="Question packs"
        description="A pack is the set of typed questions the decision model answers about a call: which option to take, and the signals that make up its risk. Each tool uses one pack."
        actions={
          canConfigure && (
            <Button variant="primary" icon={<Icon.Plus size={15} />} onClick={() => setAdding(true)}>
              New pack
            </Button>
          )
        }
      />
      <Panel>
        {q.isError && <ErrorState error={q.error} onRetry={() => void q.refetch()} />}
        {q.isPending ? (
          <LoadingPanel rows={6} />
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Pack</Th>
                <Th>Source</Th>
                <Th className="text-right">Signals</Th>
                <Th className="text-right">Guards</Th>
                <Th>Used by</Th>
              </tr>
            </thead>
            <tbody>
              {q.data?.items.map((p) => (
                <LinkRow key={p.name} href={`/packs/${p.name}`}>
                  <Td>
                    <RowLink href={`/packs/${p.name}`}>{p.title || p.name}</RowLink>
                    <div className="text-xs text-ink-3">
                      {p.name}, version {p.version}
                    </div>
                  </Td>
                  <Td>
                    {p.builtin ? (
                      p.overridden ? (
                        <Pill tone="safer">Built in, edited</Pill>
                      ) : (
                        <Pill>Built in</Pill>
                      )
                    ) : (
                      <Pill tone="held">Custom</Pill>
                    )}
                  </Td>
                  <Td className="figures text-right">{p.definition?.signals.length ?? 0}</Td>
                  <Td className="figures text-right">{p.definition?.guards.length ?? 0}</Td>
                  <Td className="max-w-80 truncate text-sm text-ink-2">
                    {p.tools?.length ? p.tools.join(", ") : <span className="text-ink-3">No tools</span>}
                  </Td>
                </LinkRow>
              ))}
            </tbody>
          </Table>
        )}
      </Panel>
      <Dialog
        open={adding}
        onClose={() => setAdding(false)}
        title="New question pack"
        description="It starts as a copy of the general pack, which you can then edit."
        width="sm"
        footer={
          <>
            <Button variant="ghost" onClick={() => setAdding(false)}>
              Cancel
            </Button>
            <Button variant="primary" disabled={!SLUG.test(name) || taken} onClick={() => router.push(`/packs/${name}?new=1` as Route)}>
              Continue
            </Button>
          </>
        }
      >
        <Field
          label="Name"
          hint={
            taken
              ? "A pack with this name exists."
              : "Lowercase letters, digits and underscores, starting with a letter. For example invoices."
          }
        >
          {(id, d) => <Input id={id} aria-describedby={d} autoFocus value={name} onChange={(e) => setName(e.target.value.trim())} />}
        </Field>
      </Dialog>
    </>
  );
}
