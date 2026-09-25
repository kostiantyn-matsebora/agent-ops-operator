import { usePipelineIcon } from '../api/hooks'
import { Icon } from './Icon'
import { PlainText } from './Text'

/**
 * A Pipeline's name with the icon it declares, wherever the console mentions
 * one: a row, a chip, a crumb, a panel. The icon comes with the name where the
 * caller has it (a list row, a graph node) and is looked up otherwise, so no
 * mention draws a different icon from the next.
 */
export function PipelineName({ name, icon }: { name: string; icon?: string }) {
  const lookup = usePipelineIcon()
  const ref = icon ?? lookup(name)
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 5 }}>
      {ref && <Icon icon={ref} />}
      <PlainText>{name}</PlainText>
    </span>
  )
}
