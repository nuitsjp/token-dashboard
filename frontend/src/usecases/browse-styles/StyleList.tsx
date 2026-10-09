import { useEffect, useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Badge, Card, Group, Image, Stack, Text } from '@mantine/core';
import { renderStylePreview, subscribeThemeData } from '../../features/display/theme-renderer';
import { getSettings } from '../../features/settings/queries';
import { loadStyleCatalog, styleCatalogRefetchInterval, type StyleDefinition } from '../../features/styles/catalog';
import { ErrorNotice } from '../../shared/ErrorNotice';

const builtins = [
  { id: 'gauges', name: 'Gauges', builtin: true },
  { id: 'bars', name: 'Bars', builtin: true },
];
const builtinIds = new Set(builtins.map(style => style.id));

function styleCards(catalog: StyleDefinition[]) {
  const extras = catalog.filter(style => !builtinIds.has(style.id))
    .sort((a, b) => a.name < b.name ? -1 : a.name > b.name ? 1 : a.id < b.id ? -1 : a.id > b.id ? 1 : 0)
    .map(style => ({ ...style, builtin: false }));
  return [...builtins, ...extras];
}

export function StyleList() {
  const settings = useQuery(getSettings());
  const catalog = useQuery({
    queryKey: ['styles', 'catalog'],
    queryFn: loadStyleCatalog,
    refetchInterval: styleCatalogRefetchInterval,
    placeholderData: previous => previous,
  });
  const cards = useMemo(() => styleCards(catalog.data ?? []), [catalog.data]);
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
  return <Stack gap="md">
    <ErrorNotice error={catalog.error ?? settings.error} />
    {cards.map(card => <Card key={card.id} component="article" aria-label={card.name} withBorder padding="md">
      <Group gap="sm" mb="sm">
        <Text fw={700}>{card.name}</Text>
        {card.builtin && <Badge variant="light" color="gray" tt="none">Built-in</Badge>}
        {card.name === inUse && <Badge variant="light" tt="none">In use</Badge>}
      </Group>
      {images[card.id]
        ? <Image src={images[card.id]} alt={`${card.name} preview`} radius="sm" style={{ aspectRatio: '1920 / 462' }} />
        : <Text size="sm" c={failures[card.id] ? 'red' : 'dimmed'}>{failures[card.id] ?? 'No image yet.'}</Text>}
    </Card>)}
  </Stack>;
}
