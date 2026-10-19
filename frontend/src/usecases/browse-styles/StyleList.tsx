import { useEffect, useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Badge, Card, Group, Image, Stack, Text } from '@mantine/core';
import { renderStylePreview, subscribeThemeData } from '../../features/display/theme-renderer';
import { getSettings } from '../../features/settings/queries';
import { compiledStyles, loadStyleCatalog, type StyleDefinition } from '../../features/styles/catalog';
import { DeleteStyle } from '../delete-style/DeleteStyle';
import { ErrorNotice } from '../../shared/ErrorNotice';

const legacyBuiltins = [
  { id: 'gauges', name: 'Gauges', builtin: true },
  { id: 'bars', name: 'Bars', builtin: true },
];
const compiled = compiledStyles();
const builtinIds = new Set(compiled.map(style => style.id));

function styleCards(catalog: StyleDefinition[], presentation: boolean) {
  const order = (a: StyleDefinition, b: StyleDefinition) => a.name < b.name ? -1 : a.name > b.name ? 1 : a.id < b.id ? -1 : a.id > b.id ? 1 : 0;
  const builtins = presentation ? [...legacyBuiltins, ...compiled.filter(style => !legacyBuiltins.some(legacy => legacy.id === style.id)).sort(order).map(style => ({ ...style, builtin: true }))] : legacyBuiltins;
  const extras = catalog.filter(style => !builtinIds.has(style.id))
    .sort(order)
    .map(style => ({ ...style, builtin: false }));
  return [...builtins, ...extras];
}

export function StyleList() {
  const settings = useQuery(getSettings());
  const catalog = useQuery({
    queryKey: ['styles', 'catalog'],
    queryFn: loadStyleCatalog,
  });
  const presentation = settings.data?.presentation != null;
  const cards = useMemo(() => styleCards(catalog.data ?? [], presentation), [catalog.data, presentation]);
  const [revision, setRevision] = useState(0);
  const [images, setImages] = useState<Record<string, string>>({});
  const [failures, setFailures] = useState<Record<string, string>>({});
  useEffect(() => subscribeThemeData(() => setRevision(value => value + 1)), []);
  useEffect(() => {
    let cancel = false;
    const current = cards;
    void (async () => {
      const next: Record<string, string> = {};
      const errors: Record<string, string> = {};
      for (const card of current) {
        try {
          next[card.id] = await renderStylePreview(card.id);
        } catch (error) {
          errors[card.id] = error instanceof Error ? error.message : String(error);
        }
        if (cancel) return;
      }
      if (!cancel) {
        setImages(next);
        setFailures(errors);
      }
    })();
    return () => { cancel = true; };
  }, [cards, revision]);
  const inUse = settings.data ? (settings.data.limitStyle || 'Gauges') : undefined;
  const selectedID = inUse === 'Gauges' || inUse === 'Bars' ? inUse.toLowerCase() : inUse;
  return <Stack gap="md">
    <ErrorNotice error={catalog.error ?? settings.error} />
    {cards.map(card => <Card key={card.id} component="article" aria-label={card.name} withBorder padding="md">
      <Group gap="sm" mb="sm" justify="space-between">
        <Group gap="sm">
        <Text fw={700}>{card.name}</Text>
        {card.builtin && <Badge variant="light" color="gray" tt="none">Built-in</Badge>}
        {card.id === selectedID && <Badge variant="light" tt="none">In use</Badge>}
        </Group>
        {!card.builtin && <DeleteStyle style={card} active={card.id === selectedID} />}
      </Group>
      {images[card.id]
        ? <Image src={images[card.id]} alt={`${card.name} preview`} radius="sm" fit="contain" style={{ width: '100%', maxHeight: 360, objectFit: 'contain' }} />
        : <Text size="sm" c={failures[card.id] ? 'red' : 'dimmed'}>{failures[card.id] ?? 'No image yet.'}</Text>}
    </Card>)}
  </Stack>;
}
