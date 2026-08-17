export type RankTier = {
	rank: string;
	label: string;
	baseRating: number;
	color: string;
};

export const RANK_TIERS: RankTier[] = [
	{ rank: 'D', label: 'Rank D (< 900)', baseRating: 800, color: 'var(--rank-d, #9e9e9e)' },
	{ rank: 'C-', label: 'Rank C- (900–999)', baseRating: 900, color: 'var(--rank-c-minus, #4caf50)' },
	{ rank: 'C', label: 'Rank C (1000–1099)', baseRating: 1000, color: 'var(--rank-c, #2196f3)' },
	{ rank: 'C+', label: 'Rank C+ (1100–1199)', baseRating: 1100, color: 'var(--rank-c-plus, #03a9f4)' },
	{ rank: 'B-', label: 'Rank B- (1200–1299)', baseRating: 1200, color: 'var(--rank-b-minus, #9c27b0)' },
	{ rank: 'B', label: 'Rank B (1300–1399)', baseRating: 1300, color: 'var(--rank-b, #673ab7)' },
	{ rank: 'B+', label: 'Rank B+ (1400–1499)', baseRating: 1400, color: 'var(--rank-b-plus, #3f51b5)' },
	{ rank: 'A-', label: 'Rank A- (1500–1599)', baseRating: 1500, color: 'var(--rank-a-minus, #ff9800)' },
	{ rank: 'A', label: 'Rank A (1600+)', baseRating: 1600, color: 'var(--rank-a, #ff5722)' }
];

export function getRank(rating: number | undefined | null): string {
	if (rating == null || isNaN(rating)) return 'C';
	if (rating < 900) return 'D';
	if (rating < 1000) return 'C-';
	if (rating < 1100) return 'C';
	if (rating < 1200) return 'C+';
	if (rating < 1300) return 'B-';
	if (rating < 1400) return 'B';
	if (rating < 1500) return 'B+';
	if (rating < 1600) return 'A-';
	return 'A';
}

export function getRankBadgeClass(rank: string): string {
	switch (rank) {
		case 'A':
		case 'A-':
			return 'badge-rank-gold';
		case 'B+':
		case 'B':
		case 'B-':
			return 'badge-rank-purple';
		case 'C+':
		case 'C':
		case 'C-':
			return 'badge-rank-blue';
		default:
			return 'badge-rank-gray';
	}
}

export function rankToBaseRating(rank: string): number {
	const tier = RANK_TIERS.find((t) => t.rank === rank);
	return tier ? tier.baseRating : 1000;
}
