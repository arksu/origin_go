import trees from './trees.json'
import animals from './animals.json'
import structures from './structures.json'
import containers from './containers.json'
import crafting from './crafting.json'
import vehicles from './vehicles.json'
import misc from './misc.json'

const objects = {
  ...trees,
  ...animals,
  ...structures,
  ...containers,
  ...crafting,
  ...vehicles,
  ...misc,
}

export default objects
