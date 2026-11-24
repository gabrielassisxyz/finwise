import { describe, expect, it } from 'vitest';
import type { FinwiseApi, ProposedChange } from '../types';

type MethodParameters<Name extends string> = Name extends `${infer Domain}.${infer Method}`
  ? Domain extends keyof FinwiseApi
    ? Method extends keyof FinwiseApi[Domain]
      ? FinwiseApi[Domain][Method] extends (...args: infer Args) => unknown
        ? Args
        : ['not a callable method', Name]
      : ['unknown method', Name]
    : ['unknown domain', Name]
  : ['invalid method name', Name];

type AssignableMember<Member extends ProposedChange> = Member extends { method: infer Name extends string; arguments: infer Args }
  ? Args extends MethodParameters<Name>
    ? true
    : ['arguments not assignable to method parameters', Name]
  : ['malformed member'];

// Compile-time check: changing any member's argument tuple in ProposedChange so it is no longer assignable to its
// method's parameters turns AssignableMember<ProposedChange> into a union containing the error tuple, failing typecheck.
type AssertTrue<Value extends true> = true extends Value ? true : never;
const ALL_MEMBERS_ASSIGNABLE: AssertTrue<AssignableMember<ProposedChange>> = true;

describe('ProposedChange', () => {
  it('has every member assignable to its method parameters', () => {
    expect(ALL_MEMBERS_ASSIGNABLE).toBe(true);
  });

  it('carries the line an agent proposes for a transaction waiting for review', () => {
    const line: ProposedChange = { method: 'transactions.setLine', arguments: ['tx-1', 'Mercado'] };
    expect(line.method).toBe('transactions.setLine');
  });
});
