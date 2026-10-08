// A stand-in name for an account an operator saved without one. An unnamed
// account is captioned by its hex id on every usage surface, which reads as
// machine noise; a memorable pair of words at least tells two rows apart at a
// glance.

const ANIMALS = [
	'Aardvark',
	'Badger',
	'Beaver',
	'Bison',
	'Camel',
	'Capybara',
	'Cheetah',
	'Chinchilla',
	'Cobra',
	'Condor',
	'Coyote',
	'Crane',
	'Dingo',
	'Dolphin',
	'Dormouse',
	'Eagle',
	'Echidna',
	'Falcon',
	'Ferret',
	'Finch',
	'Flamingo',
	'Gazelle',
	'Gecko',
	'Gerbil',
	'Gibbon',
	'Hedgehog',
	'Heron',
	'Ibis',
	'Jackal',
	'Jaguar',
	'Kestrel',
	'Kingfisher',
	'Koala',
	'Lemur',
	'Leopard',
	'Lynx',
	'Marmot',
	'Meerkat',
	'Mongoose',
	'Narwhal',
	'Ocelot',
	'Otter',
	'Pangolin',
	'Panther',
	'Puffin',
	'Quokka',
	'Rabbit',
	'Raven',
	'Salamander',
	'Sparrow'
];

const PERSONALITIES = [
	'Adaptable',
	'Adventurous',
	'Agreeable',
	'Alert',
	'Ambitious',
	'Amiable',
	'Bold',
	'Brave',
	'Brisk',
	'Calm',
	'Candid',
	'Cheerful',
	'Clever',
	'Composed',
	'Confident',
	'Courageous',
	'Creative',
	'Curious',
	'Daring',
	'Eager',
	'Energetic',
	'Fair',
	'Fierce',
	'Focused',
	'Friendly',
	'Gentle',
	'Graceful',
	'Grounded',
	'Honest',
	'Humble',
	'Independent',
	'Jolly',
	'Keen',
	'Kind',
	'Lively',
	'Loyal',
	'Methodical',
	'Optimistic',
	'Patient',
	'Pensive',
	'Playful',
	'Practical',
	'Quick',
	'Quiet',
	'Resolute',
	'Resourceful',
	'Sincere',
	'Steady',
	'Thoughtful',
	'Witty'
];

/** The words a generated name is drawn from. Both lists are 50 long, which leaves
 *  2500 pairs. */
export const NAME_WORDS = { animals: ANIMALS, personalities: PERSONALITIES } as const;

/** Names an account that was saved without one, as an animal and a personality.
 *  The caller passes a random source so a name can be pinned in a test. */
export function defaultAccountName(random: () => number = Math.random): string {
	const animal = ANIMALS[Math.floor(random() * ANIMALS.length)];
	const personality = PERSONALITIES[Math.floor(random() * PERSONALITIES.length)];
	return animal + ' ' + personality;
}
