import { useTranslations } from 'next-intl';
import { notFound } from 'next/navigation';

const KNOWN_COMPETITORS = ['verkada', 'rhombus', 'milestone', 'avigilon'] as const;
type Competitor = (typeof KNOWN_COMPETITORS)[number];

export function generateStaticParams() {
  return KNOWN_COMPETITORS.map((competitor) => ({ competitor }));
}

export default function ComparePage({
  params: { competitor }
}: {
  params: { competitor: string };
}) {
  if (!KNOWN_COMPETITORS.includes(competitor as Competitor)) notFound();

  const t = useTranslations('Compare');
  const display = competitor.charAt(0).toUpperCase() + competitor.slice(1);

  return (
    <section>
      <h1>{t('title', { competitor: display })}</h1>
      <p>{t('subtitle', { competitor: display })}</p>
      <p>{t('placeholder')}</p>
    </section>
  );
}
