export type ConfigSectionId = 'cofrinhos' | 'apelidos' | 'categorizacao' | 'cartao' | 'pin' | 'dados';

export const CONFIG_SECTIONS: { id: ConfigSectionId; label: string }[] = [
  { id: 'cofrinhos', label: 'Cofrinhos' },
  { id: 'apelidos', label: 'Apelidos' },
  { id: 'categorizacao', label: 'Categorização' },
  { id: 'cartao', label: 'Cartão' },
  { id: 'pin', label: 'PIN' },
  { id: 'dados', label: 'Dados' },
];

/** A path that names no section opens the first one. */
export function sectionOf(param: unknown): ConfigSectionId {
  return CONFIG_SECTIONS.find((section) => section.id === param)?.id ?? 'cofrinhos';
}
